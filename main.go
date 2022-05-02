package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
)

type Zone struct {
	UUID    string   `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Records []Record `json:"records"`
}

type Record struct {
	UUID     string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Content  string `json:"content,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	Priority int    `json:"prio"`
	Disabled bool   `json:"disabled"`
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&ionosDNSProviderSolver{},
	)
}

type ionosDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type ionosDNSProviderConfig struct {
	Endpoint         string                   `json:"endpoint"`
	AuthAPIKey       string                   `json:"authApiKey"`
	AuthAPISecretRef corev1.SecretKeySelector `json:"authApiSecretRef"`
	BaseURL          string                   `json:"baseUrl"`
	TTL              int                      `json:"ttl"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *ionosDNSProviderSolver) Name() string {
	return "ionos"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *ionosDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	record := &Record{
		Name:     ch.ResolvedFQDN,
		Type:     "TXT",
		Content:  ch.Key,
		TTL:      *&cfg.TTL,
		Priority: 0,
		Disabled: false,
	}

	recordArray := []Record{*record}

	jsonValue, err := json.Marshal(recordArray)
	body := bytes.NewBuffer(jsonValue)

	fmt.Println(body)

	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, ch.Endpoint+"/v1/zones/"+ch.ResolvedZone+"/records", body)

	if err != nil {
		return err
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	if err != nil || resp.StatusCode != 201 {
		return
	}

	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *ionosDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodDelete, cfg.BaseURL+"/v1/zones/"+zone+"/records/"+record_id, nil)

	if err != nil {
		return err
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	fmt.Println(resp.StatusCode)

	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	return nil
}

func (c *ionosDNSProviderSolver) validateAndGetSecret(cfg *ionosDNSProviderConfig, namespace string) (string, error) {
	fmt.Printf("validateAndGetSecret...")
	// Check that the host is defined
	if cfg.AuthAPIKey != "" {
		return cfg.AuthAPIKey, nil
	}

	// Try to load the API key
	if cfg.AuthAPISecretRef.LocalObjectReference.Name == "" {
		return "", errors.New("No Arvan API secret provided")
	}

	sec, err := c.client.CoreV1().Secrets(namespace).Get(context.TODO(), cfg.AuthAPISecretRef.LocalObjectReference.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	secBytes, ok := sec.Data[cfg.AuthAPISecretRef.Key]
	if !ok {
		return "", fmt.Errorf("Key %q not found in secret \"%s/%s\"", cfg.AuthAPISecretRef.Key, cfg.AuthAPISecretRef.LocalObjectReference.Name, namespace)
	}

	apiKey := string(secBytes)

	return apiKey, nil
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *ionosDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = cl

	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (ionosDNSProviderConfig, error) {
	cfg := ionosDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

const HEADER_NAME = "X-API-Key"

func (c *ionosDNSProviderSolver) findManagedZoneID(cfg *ionosDNSProviderConfig, domain string, namespace string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.BaseURL+"/v1/zones", nil)
	if err != nil {
		return "", err
	}

	secret, _ := c.validateAndGetSecret(cfg, namespace)
	req.Header.Add(HEADER_NAME, cfg.AuthAPIKey+"."+secret)

	client := new(http.Client)
	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	zones := make([]Zone, 0)

	err = json.NewDecoder(resp.Body).Decode(&zones)

	if err != nil {
		return "", err
	}

	for _, element := range zones {
		if element.Name == domain {
			return element.UUID, nil
		}
	}

	return "", errors.New("domain not available in account")
}
