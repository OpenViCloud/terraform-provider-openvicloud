package main

// Acceptance tests run against a real OpenViCloud API (no fake): TF_ACC=1 with OPENVICLOUD_API_URL and
// OPENVICLOUD_TOKEN set, e.g. the dev cluster with the token of lima/scripts/tf-acc-token.sh of OpenViCloud/titan-local-cluster.
// Every resource they create is named tfacc-<random> and checked gone at the end.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"openvicloud": providerserver.NewProtocol6WithError(provider.New("test", DataSources(), Resources())()),
}

func cluster() string {
	if value := os.Getenv("OPENVICLOUD_TEST_CLUSTER"); value != "" {
		return value
	}
	return "local"
}

func preCheck(t *testing.T) {
	if os.Getenv("OPENVICLOUD_TOKEN") == "" || os.Getenv("OPENVICLOUD_API_URL") == "" {
		t.Fatal("OPENVICLOUD_TOKEN and OPENVICLOUD_API_URL must be set for acceptance tests")
	}
}

// oracle is a client of the same API, used to check what the provider did independently of the provider's state.
func oracle(t *testing.T) *api.ClientWithResponses {
	client, err := api.New(os.Getenv("OPENVICLOUD_API_URL"), os.Getenv("OPENVICLOUD_TOKEN"), "acceptance", &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAccCatalog_ListsWhatTheApiLists(t *testing.T) {
	client := oracle(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "openvicloud_clusters" "all" {}
data "openvicloud_templates" "here" { cluster_id = %q }
data "openvicloud_vm_offerings" "all" {}
data "openvicloud_volume_offerings" "all" {}
`, cluster()),
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						clusters, _ := client.TitanInfrastructureEndpointsCatalogListClustersEndpointWithResponse(context.Background())
						z := cluster()
						templates, _ := client.TitanInfrastructureEndpointsCatalogListTemplatesEndpointWithResponse(context.Background(),
							&api.TitanInfrastructureEndpointsCatalogListTemplatesEndpointParams{ClusterId: &z})
						vms, _ := client.TitanInfrastructureEndpointsCatalogListVmOfferingsEndpointWithResponse(context.Background())
						volumes, _ := client.TitanInfrastructureEndpointsCatalogListVolumeOfferingsEndpointWithResponse(context.Background())
						want := map[string]int{
							"data.openvicloud_clusters.all.clusters.#":                 len(*clusters.JSON200),
							"data.openvicloud_templates.here.templates.#":              len(*templates.JSON200),
							"data.openvicloud_vm_offerings.all.vm_offerings.#":         len(*vms.JSON200),
							"data.openvicloud_volume_offerings.all.volume_offerings.#": len(*volumes.JSON200),
						}
						for key, count := range want {
							if count == 0 {
								return fmt.Errorf("the API lists nothing for %s: the comparison would be empty with empty", key)
							}
							parts := regexp.MustCompile(`^(data\.[a-z_]+\.[a-z]+)\.(.+)$`).FindStringSubmatch(key)
							got := s.RootModule().Resources[parts[1]].Primary.Attributes[parts[2]]
							if got != fmt.Sprint(count) {
								return fmt.Errorf("%s = %s, the API lists %d", key, got, count)
							}
						}
						return nil
					},
				),
			},
			{
				Config:      `data "openvicloud_templates" "nowhere" { cluster_id = "no-such-cluster" }`,
				ExpectError: regexp.MustCompile(`CLUSTER_NOT_FOUND`),
			},
		},
	})
}

func networkConfig(name string) string {
	return fmt.Sprintf(`
resource "openvicloud_network" "n" {
  name    = %q
  cluster_id = %q
}
resource "openvicloud_firewall_rule" "ssh" {
  network_id = openvicloud_network.n.id
  direction  = "Ingress"
  protocol   = "TCP"
  start_port = 22
  end_port   = 22
  cidr       = "0.0.0.0/0"
}
resource "openvicloud_firewall_rule" "ping" {
  network_id = openvicloud_network.n.id
  direction  = "Ingress"
  protocol   = "ICMP"
  cidr       = "0.0.0.0/0"
}
`, name, cluster())
}

func TestAccNetworkAndRules_CreateImportRenameDestroy(t *testing.T) {
	client := oracle(t)
	name := "tfacc-" + acctest.RandString(8)
	var networkId string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             func(s *terraform.State) error { return leftovers(client, name) },
		Steps: []resource.TestStep{
			{
				Config: networkConfig(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_network.n", "name", name),
					resource.TestCheckResourceAttrSet("openvicloud_network.n", "cidr"),
					resource.TestCheckResourceAttrSet("openvicloud_network.n", "gateway"),
					resource.TestCheckResourceAttr("openvicloud_firewall_rule.ssh", "start_port", "22"),
					resource.TestCheckNoResourceAttr("openvicloud_firewall_rule.ping", "start_port"),
					func(s *terraform.State) error {
						networkId = s.RootModule().Resources["openvicloud_network.n"].Primary.ID
						rules, err := client.TitanInfrastructureEndpointsNetworkListFirewallRulesEndpointWithResponse(context.Background(), networkId)
						if err != nil {
							return err
						}
						if len(*rules.JSON200) != 2 {
							return fmt.Errorf("the API lists %d rules on the network, want 2", len(*rules.JSON200))
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "openvicloud_network.n",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName: "openvicloud_firewall_rule.ssh",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rule := s.RootModule().Resources["openvicloud_firewall_rule.ssh"].Primary
					return rule.Attributes["network_id"] + "/" + rule.ID, nil
				},
				ImportStateVerify: true,
			},
			{
				Config: networkConfig(name + "-renamed"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_network.n", "name", name+"-renamed"),
					func(s *terraform.State) error {
						if id := s.RootModule().Resources["openvicloud_network.n"].Primary.ID; id != networkId {
							return fmt.Errorf("rename replaced the network: id %s, was %s", id, networkId)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccFirewallRule_PortsAreCheckedAtPlan(t *testing.T) {
	rule := func(protocol string, ports string) string {
		return fmt.Sprintf(`
resource "openvicloud_firewall_rule" "r" {
  network_id = "00000000-0000-4000-8000-000000000000"
  direction  = "Ingress"
  protocol   = %q
  cidr       = "0.0.0.0/0"
  %s
}`, protocol, ports)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: rule("ICMP", "start_port = 1\n  end_port = 1"), PlanOnly: true, ExpectError: regexp.MustCompile(`take no start_port`)},
			{Config: rule("TCP", ""), PlanOnly: true, ExpectError: regexp.MustCompile(`need start_port and end_port`)},
			{Config: rule("UDP", "start_port = 90\n  end_port = 80"), PlanOnly: true, ExpectError: regexp.MustCompile(`at least start_port`)},
		},
	})
}
