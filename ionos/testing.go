package ionos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

const HEADER_NAME = "X-API-Key"
const API_HOST = "https://api.hosting.ionos.com/dns"

const PREFIX = "2d7648e10caf47d4b228bb16a82f3592"
const SECRET = "GgNjuBgVNN53---baLvnJnJ9eH7_BDTU_SfP1c0rL5bKElbR93YpGndDwLu1CZGDQ1pDirdMZykFNCpt58ulGg"

var zoneName = "code-forge.eu"

func createRecord(zone string, key string, value string) {
	record := &Record{
		Name:     key,
		Type:     "TXT",
		Content:  value,
		TTL:      60,
		Priority: 0,
		Disabled: false,
	}

	recordArray := []Record{*record}

	jsonValue, err := json.Marshal(recordArray)
	body := bytes.NewBuffer(jsonValue)

	fmt.Println(body)

	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, API_HOST+"/v1/zones/"+zone+"/records", body)

	if err != nil {
		return
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	if err != nil || resp.StatusCode != 201 {
		return
	}
}

func updateRecord(zone string, record_id string, value string) {
	record := &Record{
		Content:  value,
		TTL:      60,
		Priority: 0,
		Disabled: false,
	}

	jsonValue, err := json.Marshal(record)
	body := bytes.NewBuffer(jsonValue)

	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPut, API_HOST+"/v1/zones/"+zone+"/records/"+record_id, body)

	if err != nil {
		return
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	if err != nil || resp.StatusCode != 200 {
		return
	}
}

func deleteRecord(zone string, record_id string) {
	req, err := http.NewRequest(http.MethodDelete, API_HOST+"/v1/zones/"+zone+"/records/"+record_id, nil)

	if err != nil {
		return
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	fmt.Println(resp.StatusCode)

	if err != nil || resp.StatusCode != 200 {
		return
	}
}

func findManagedZoneID(name string) string {
	req, err := http.NewRequest(http.MethodGet, API_HOST+"/v1/zones", nil)
	if err != nil {
		return ""
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)

	client := new(http.Client)
	resp, err := client.Do(req)

	if err != nil {
		return ""
	}

	defer resp.Body.Close()

	zones := make([]Zone, 0)

	err = json.NewDecoder(resp.Body).Decode(&zones)

	if err != nil {
		return ""
	}

	for _, element := range zones {
		if element.Name == name {
			return element.UUID
		}
	}

	for _, element := range zones {
		if strings.HasSuffix(element.Name, name) {
			return element.UUID
		}
	}

	return ""
}

func getRecords(zoneId string, name string) []Record {
	req, err := http.NewRequest(http.MethodGet, API_HOST+"/v1/zones/"+zoneId, nil)

	if err != nil {
		return nil
	}

	req.Header.Add(HEADER_NAME, PREFIX+"."+SECRET)
	req.Header.Add("Accept", "application/json")

	client := new(http.Client)

	resp, err := client.Do(req)

	if err != nil {
		return nil
	}

	defer resp.Body.Close()

	zone := new(Zone)
	err = json.NewDecoder(resp.Body).Decode(zone)

	if err != nil {
		return nil
	}

	record := make([]Record, 0)

	for _, element := range zone.Records {
		if element.Type != "TXT" || element.Name != name {
			continue
		}

		record = append(record, element)
	}

	return record
}
