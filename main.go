package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const HEADER_NAME = "X-API-Key"

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
	// TODO: cache zone id
	client *kubernetes.Clientset
}

type ionosDNSProviderConfig struct {
	TTL              int                      `json:"ttl"`
	Endpoint         string                   `json:"host"`
	APIKey           string                   `josn:"apiKey"`
	AuthAPISecretRef corev1.SecretKeySelector `json:"apiKeySecretRef"`
}

func (c *ionosDNSProviderSolver) Name() string {
	return "ionos"
}

func (c *ionosDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	zone_id, err := c.getZoneId(cfg, ch)
	if err != nil {
		return err
	}

	records, err := c.listRecords(cfg, ch, zone_id)
	if err != nil {
		return err
	}

	var record_id = ""
	var is_present = false

	for _, record := range records {
		// check if key is present
		if record.Name+"." != ch.ResolvedFQDN {
			continue
		}
		record_id = record.UUID

		// check if value is already set
		// IONOS double quotes TXT record content
		if record.Content != "\""+ch.Key+"\"" {
			continue
		}

		is_present = true
		break
	}

	if record_id != "" && is_present == true {
		fmt.Println("challenge already set")
		return nil
	}

	if record_id != "" {
		fmt.Println("challenge data must be updated")
		c.updateRecord(cfg, ch, zone_id, record_id)
		return nil
	}

	fmt.Println("challenge data must be set")
	c.createRecord(cfg, ch, zone_id)

	return nil
}

func (c *ionosDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	zone_id, err := c.getZoneId(cfg, ch)
	if err != nil {
		return err
	}

	records, err := c.listRecords(cfg, ch, zone_id)
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Name+"." != ch.ResolvedFQDN {
			continue
		}

		fmt.Println("deleting record " + record.UUID)
		err = c.deleteRecord(cfg, ch, zone_id, record.UUID)

		if err != nil {
			return err
		}
	}

	return nil
}

func (c *ionosDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = cl

	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (ionosDNSProviderConfig, error) {
	cfg := ionosDNSProviderConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

func (c ionosDNSProviderSolver) getApiKey(cfg ionosDNSProviderConfig, namespace string) (string, error) {
	// api key specified directly
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}

	// api key specified using secrets (recommended)
	if cfg.AuthAPISecretRef.LocalObjectReference.Name == "" {
		return "", errors.New("no secret provided")
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

func (c *ionosDNSProviderSolver) getZoneId(cfg ionosDNSProviderConfig, ch *v1alpha1.ChallengeRequest) (string, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.Endpoint+"/v1/zones", nil)
	if err != nil {
		return "", err
	}

	secret, err := c.getApiKey(cfg, ch.ResourceNamespace)
	if err != nil {
		return "", err
	}

	req.Header.Add(HEADER_NAME, secret)

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
		if element.Name+"." == ch.ResolvedZone {
			return element.UUID, nil
		}
	}

	return "", errors.New("domain not available in account")
}

func (c ionosDNSProviderSolver) listRecords(cfg ionosDNSProviderConfig, ch *v1alpha1.ChallengeRequest, zoneId string) ([]Record, error) {
	secret, err := c.getApiKey(cfg, ch.ResourceNamespace)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/zones/%s", cfg.Endpoint, zoneId), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add(HEADER_NAME, secret)
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("invalid http response: %d", resp.StatusCode))
	}

	defer resp.Body.Close()

	zone := new(Zone)
	err = json.NewDecoder(resp.Body).Decode(zone)

	if err != nil {
		return nil, err
	}

	record := make([]Record, 0)

	for _, element := range zone.Records {
		if element.Type != "TXT" || element.Name+"." != ch.ResolvedFQDN {
			continue
		}

		record = append(record, element)
	}

	return record, nil
}

func (c ionosDNSProviderSolver) createRecord(cfg ionosDNSProviderConfig, ch *v1alpha1.ChallengeRequest, zoneId string) error {
	secret, err := c.getApiKey(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	record := &Record{
		Name:     ch.ResolvedFQDN,
		Type:     "TXT",
		Content:  ch.Key,
		TTL:      cfg.TTL,
		Priority: 0,
		Disabled: false,
	}

	recordArray := []Record{*record}
	jsonValue, err := json.Marshal(recordArray)
	if err != nil {
		return err
	}

	body := bytes.NewBuffer(jsonValue)
	req, err := http.NewRequest(http.MethodPost, cfg.Endpoint+"/v1/zones/"+zoneId+"/records", body)
	if err != nil {
		return err
	}

	req.Header.Add(HEADER_NAME, secret)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 201 {
		return errors.New(fmt.Sprintf("invalid http response: %d", resp.StatusCode))
	}

	return nil
}

func (c ionosDNSProviderSolver) updateRecord(cfg ionosDNSProviderConfig, ch *v1alpha1.ChallengeRequest, zone_id string, record_id string) error {
	secret, err := c.getApiKey(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	record := &Record{
		Content:  ch.Key,
		TTL:      60,
		Priority: 0,
		Disabled: false,
	}

	jsonValue, err := json.Marshal(record)
	body := bytes.NewBuffer(jsonValue)

	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, cfg.Endpoint+"/v1/zones/"+zone_id+"/records/"+record_id, body)

	if err != nil {
		return err
	}

	req.Header.Add(HEADER_NAME, secret)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("invalid http response: %d", resp.StatusCode))
	}

	return nil
}

func (c ionosDNSProviderSolver) deleteRecord(cfg ionosDNSProviderConfig, ch *v1alpha1.ChallengeRequest, zone_id string, record_id string) error {
	secret, err := c.getApiKey(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodDelete, cfg.Endpoint+"/v1/zones/"+zone_id+"/records/"+record_id, nil)
	if err != nil {
		return err
	}

	req.Header.Add(HEADER_NAME, secret)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("invalid http response: %d", resp.StatusCode))
	}

	return nil
}
