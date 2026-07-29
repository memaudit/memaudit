// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultCacheTTL = 60 * time.Second

// PodMeta is the pod metadata attached to an enriched cgroup_mem record.
type PodMeta struct {
	Namespace string
	Name      string
	// Labels only contains the keys allowlisted in KubeletClientConfig.LabelKeys.
	Labels map[string]string
}

// Client resolves a pod UID to metadata. KubeletClient is the only
// implementation; it exists as an interface so the cgroup collector
// doesn't need a live kubelet to be tested.
type Client interface {
	LookupPod(ctx context.Context, uid string) (PodMeta, bool, error)
}

// KubeletClientConfig configures a KubeletClient.
type KubeletClientConfig struct {
	// BaseURL is the kubelet's read/write API, e.g. "https://127.0.0.1:10250".
	BaseURL string
	// Token is the ServiceAccount bearer token used to authenticate to
	// the kubelet. Empty disables the Authorization header.
	Token string
	// CAPath verifies the kubelet's TLS certificate. Ignored if
	// InsecureSkipVerify is true.
	CAPath string
	// InsecureSkipVerify skips kubelet TLS verification entirely. Only
	// meant for local/dev clusters using self-signed certs.
	InsecureSkipVerify bool
	// LabelKeys allowlists which pod label keys are attached to
	// enriched records; every other label is dropped to keep
	// cardinality sane.
	LabelKeys []string
	// HTTPClient overrides the client used to reach the kubelet;
	// defaults to one built from CAPath/InsecureSkipVerify. Tests point
	// this at an httptest.Server.
	HTTPClient *http.Client
	// CacheTTL bounds how often the full pod list is refetched.
	// Defaults to 60s.
	CacheTTL time.Duration
}

// KubeletClient resolves pod UIDs via the kubelet's /pods endpoint,
// caching the full pod list for CacheTTL to keep enrichment cheap on a
// collector that runs every 30s against a node with hundreds of pods.
type KubeletClient struct {
	baseURL    string
	token      string
	labelKeys  map[string]struct{}
	httpClient *http.Client
	cacheTTL   time.Duration
	now        func() time.Time

	mu       sync.Mutex
	cache    map[string]PodMeta
	cachedAt time.Time
}

// NewKubeletClient builds a KubeletClient from cfg.
func NewKubeletClient(cfg KubeletClientConfig) (*KubeletClient, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport, err := kubeletTransport(cfg.CAPath, cfg.InsecureSkipVerify)
		if err != nil {
			return nil, err
		}
		httpClient = &http.Client{Transport: transport, Timeout: 10 * time.Second}
	}

	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}

	allow := make(map[string]struct{}, len(cfg.LabelKeys))
	for _, k := range cfg.LabelKeys {
		allow[k] = struct{}{}
	}

	return &KubeletClient{
		baseURL:    strings.TrimSuffix(cfg.BaseURL, "/"),
		token:      cfg.Token,
		labelKeys:  allow,
		httpClient: httpClient,
		cacheTTL:   ttl,
		now:        time.Now,
	}, nil
}

// kubeletTransport builds the TLS transport used to reach the kubelet.
func kubeletTransport(caPath string, insecure bool) (*http.Transport, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // G402: operator opt-in via K8sConfig.InsecureSkipVerify, for self-signed dev clusters only
	if !insecure && caPath != "" {
		caCert, err := os.ReadFile(caPath) //nolint:gosec // G304: path is operator-supplied via config, not untrusted input
		if err != nil {
			return nil, fmt.Errorf("read kubelet CA %s: %w", caPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse kubelet CA %s: no certificates found", caPath)
		}
		tlsConfig.RootCAs = pool
	}
	return &http.Transport{TLSClientConfig: tlsConfig}, nil
}

// LookupPod resolves uid against the cached pod list, refreshing the
// cache first if it's stale.
func (c *KubeletClient) LookupPod(ctx context.Context, uid string) (PodMeta, bool, error) {
	if err := c.refreshIfStale(ctx); err != nil {
		return PodMeta{}, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	meta, ok := c.cache[uid]
	return meta, ok, nil
}

func (c *KubeletClient) refreshIfStale(ctx context.Context) error {
	c.mu.Lock()
	stale := c.cache == nil || c.now().Sub(c.cachedAt) >= c.cacheTTL
	c.mu.Unlock()
	if !stale {
		return nil
	}

	items, err := c.fetchPods(ctx)
	if err != nil {
		return err
	}

	cache := make(map[string]PodMeta, len(items))
	for _, item := range items {
		cache[item.Metadata.UID] = PodMeta{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			Labels:    filterLabels(item.Metadata.Labels, c.labelKeys),
		}
	}

	c.mu.Lock()
	c.cache = cache
	c.cachedAt = c.now()
	c.mu.Unlock()
	return nil
}

type podList struct {
	Items []podItem `json:"items"`
}

type podItem struct {
	Metadata podMetadata `json:"metadata"`
}

type podMetadata struct {
	UID       string            `json:"uid"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
}

func (c *KubeletClient) fetchPods(ctx context.Context) ([]podItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/pods", nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s/pods: %w", c.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s/pods: %s", c.baseURL, resp.Status)
	}

	var list podList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode kubelet /pods response: %w", err)
	}
	return list.Items, nil
}

// filterLabels returns labels restricted to allow, or nil if nothing
// survives the filter.
func filterLabels(labels map[string]string, allow map[string]struct{}) map[string]string {
	if len(labels) == 0 || len(allow) == 0 {
		return nil
	}
	out := make(map[string]string, len(allow))
	for k := range allow {
		if v, ok := labels[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
