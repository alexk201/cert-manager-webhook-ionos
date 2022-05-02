package ionos

import (
	"fmt"
	"testing"
)

const domain = "code-forge.eu"
const challenge_name = "_acme-challenge.proxmox.code-forge.eu"
const challenge_data = "pWXImILm_tmQQguhdzUQD_lDiTcMyam9j4NpmzX_Byg"

func CleanupTest(t *testing.T) {
	zone_id := findManagedZoneID(domain)
	records := getRecords(zone_id, challenge_name)

	for _, record := range records {
		if record.Name != challenge_name {
			continue
		}

		fmt.Print("deleting record " + record.UUID)
		deleteRecord(zone_id, record.UUID)
	}
}

func CreateTest(t *testing.T) {
	zone_id := findManagedZoneID(domain)
	records := getRecords(zone_id, challenge_name)

	var record_id = ""
	var is_present = false

	for _, record := range records {
		// check if key is present
		if record.Name != challenge_name {
			continue
		}
		record_id = record.UUID

		// check if value is already set
		// IONOS double quotes TXT record content
		if record.Content != "\""+challenge_data+"\"" {
			continue
		}

		is_present = true
		break
	}

	if record_id != "" && is_present == true {
		fmt.Println("challenge already set")
		return
	}

	if record_id != "" {
		fmt.Println("challenge data must be updated")
		updateRecord(zone_id, record_id, challenge_data)
		return
	}

	fmt.Println("challenge data must be set")
	createRecord(zone_id, challenge_name, challenge_data)
}
