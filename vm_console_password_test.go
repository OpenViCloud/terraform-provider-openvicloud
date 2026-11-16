package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// fakeVmApi answers the /v1 routes a VM's create, read and delete use, from memory: Terraform itself plans and applies
// the provider without an OpenViCloud API. Every VM it makes is Running at once, and gone at once after a delete.
type fakeVmApi struct {
	mu      sync.Mutex
	server  *httptest.Server
	creates []map[string]any
	vms     map[string]map[string]any
}

func newFakeVmApi(t *testing.T) *fakeVmApi {
	f := &fakeVmApi{vms: map[string]map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tenant/vms", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.creates = append(f.creates, body)
		id := fmt.Sprintf("5b0e7f4c-0000-4000-8000-%012d", len(f.creates))
		f.vms[id] = body
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"vmId": id})
	})
	mux.HandleFunc("GET /v1/tenant/vms/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		body, alive := f.vms[r.PathValue("id")]
		f.mu.Unlock()
		if !alive {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"status":404,"title":"VM not found.","code":"VM_NOT_FOUND"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": r.PathValue("id"), "name": body["name"], "clusterId": body["clusterId"], "networkId": body["networkId"],
			"templateId": body["templateId"], "vmOfferingId": body["vmOfferingId"], "diskSizeGb": 10, "status": "Running",
		})
	})
	mux.HandleFunc("DELETE /v1/tenant/vms/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		delete(f.vms, r.PathValue("id"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// lastCreate is the body of the latest POST /v1/tenant/vms.
func (f *fakeVmApi) lastCreate() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.creates) == 0 {
		return nil
	}
	return f.creates[len(f.creates)-1]
}

// madeOutsideTerraform adds a VM the way the portal or the CLI would, for a configuration to import.
func (f *fakeVmApi) madeOutsideTerraform(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := "5b0e7f4c-0000-4000-8000-0000000000ff"
	f.vms[id] = map[string]any{
		"name": name, "clusterId": "local", "networkId": "9a1d0000-0000-4000-8000-000000000001",
		"templateId": "tpl-ubuntu", "vmOfferingId": "s",
	}
	return id
}

func consolePasswordConfig(url string, password string) string {
	line := ""
	if password != "" {
		line = fmt.Sprintf("console_password = %q", password)
	}
	return fmt.Sprintf(`
provider "openvicloud" {
  api_url = %q
  token   = "ovc_unit_test"
}

resource "openvicloud_vm" "v" {
  name           = "web"
  cluster_id     = "local"
  network_id     = "9a1d0000-0000-4000-8000-000000000001"
  template_id    = "tpl-ubuntu"
  vm_offering_id = "s"
  %s
}
`, url, line)
}

func sentConsolePassword(f *fakeVmApi, want any) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		body := f.lastCreate()
		if body == nil {
			return fmt.Errorf("no create reached the API")
		}
		if body["consolePassword"] != want {
			return fmt.Errorf("the create sent consolePassword %v, want %v", body["consolePassword"], want)
		}
		return nil
	}
}

// titan-ops features/vm/design.md#console-password: the password goes to the API once, at create; the plan never
// shows it, and a new password is a new VM.
func TestVmConsolePassword_IsSentHidden_AndAChangeReplacesTheVm(t *testing.T) {
	f := newFakeVmApi(t)
	password := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectSensitiveValue("openvicloud_vm.v", tfjsonpath.New("console_password")),
	}}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config:           consolePasswordConfig(f.server.URL, "correct horse battery"),
				ConfigPlanChecks: password,
				Check:            sentConsolePassword(f, "correct horse battery"),
			},
			{
				Config: consolePasswordConfig(f.server.URL, "another long password"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectSensitiveValue("openvicloud_vm.v", tfjsonpath.New("console_password")),
					plancheck.ExpectResourceAction("openvicloud_vm.v", plancheck.ResourceActionReplace),
				}},
				Check: sentConsolePassword(f, "another long password"),
			},
			{
				Config: consolePasswordConfig(f.server.URL, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("openvicloud_vm.v", plancheck.ResourceActionReplace),
				}},
				Check: sentConsolePassword(f, nil),
			},
			{
				Config: consolePasswordConfig(f.server.URL, "correct horse battery"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("openvicloud_vm.v", plancheck.ResourceActionReplace),
				}},
				Check: sentConsolePassword(f, "correct horse battery"),
			},
		},
	})
}

// The API never returns the password, so a VM with no password in state (imported, or made by a provider version
// without this attribute) only records the configured one: a replacement would destroy the customer's VM.
func TestVmConsolePassword_OfAnImportedVm_IsRecordedInPlace_NotReplaced(t *testing.T) {
	f := newFakeVmApi(t)
	id := f.madeOutsideTerraform("web")
	config := fmt.Sprintf(`
import {
  to = openvicloud_vm.v
  id = %q
}
`, id) + consolePasswordConfig(f.server.URL, "correct horse battery")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("openvicloud_vm.v", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_vm.v", "id", id),
					func(_ *terraform.State) error {
						if f.lastCreate() != nil {
							return fmt.Errorf("the import created a VM: %v", f.lastCreate())
						}
						return nil
					},
				),
			},
		},
	})
}

func TestVmConsolePassword_OutsideTwelveToSeventyTwo_IsRefusedAtPlan(t *testing.T) {
	f := newFakeVmApi(t)
	for _, password := range []string{strings.Repeat("a", 11), strings.Repeat("a", 73)} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			Steps: []resource.TestStep{{
				Config:      consolePasswordConfig(f.server.URL, password),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`between 12 and 72`),
			}},
		})
	}
	if f.lastCreate() != nil {
		t.Fatalf("a create reached the API: %v", f.lastCreate())
	}
}
