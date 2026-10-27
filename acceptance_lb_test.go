package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// lbConfig is a network with public IPv6, two VMs and a load balancer sending to the VMs listed in backends.
func lbConfig(name, lbName, listeners, backends string) string {
	return fmt.Sprintf(`
data "openvicloud_templates" "here" { cluster_id = %[3]q }

resource "openvicloud_network" "n" {
  name               = %[1]q
  cluster_id            = %[3]q
  enable_public_ipv6 = true
}

resource "openvicloud_vm" "a" {
  name           = "%[1]s-a"
  cluster_id        = %[3]q
  network_id     = openvicloud_network.n.id
  template_id    = data.openvicloud_templates.here.templates[0].id
  vm_offering_id = "s"
}

resource "openvicloud_vm" "b" {
  name           = "%[1]s-b"
  cluster_id        = %[3]q
  network_id     = openvicloud_network.n.id
  template_id    = data.openvicloud_templates.here.templates[0].id
  vm_offering_id = "s"
}

resource "openvicloud_load_balancer" "lb" {
  name           = %[2]q
  network_id     = openvicloud_network.n.id
  listener       = [%[4]s]
  backend_vm_ids = [%[5]s]
}
`, name, lbName, cluster(), listeners, backends)
}

func TestAccLoadBalancer_CreateImportUpdateInPlaceDestroy(t *testing.T) {
	client := oracle(t)
	name := "tfacc-" + acctest.RandString(8)
	http := `{ protocol = "TCP", port = 80, target_port = 8080 }`
	dns := `{ protocol = "UDP", port = 53, target_port = 53 }`
	var lbId, vip string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             func(_ *terraform.State) error { return leftovers(client, name) },
		Steps: []resource.TestStep{
			{
				Config: lbConfig(name, name, http, "openvicloud_vm.a.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_load_balancer.lb", "listener.#", "1"),
					resource.TestCheckResourceAttrSet("openvicloud_load_balancer.lb", "vip"),
					func(s *terraform.State) error {
						lb := s.RootModule().Resources["openvicloud_load_balancer.lb"].Primary
						lbId, vip = lb.ID, lb.Attributes["vip"]
						found, err := client.TitanInfrastructureEndpointsLoadBalancerGetLoadBalancerEndpointWithResponse(context.Background(), lbId)
						if err != nil {
							return err
						}
						onIncus := found.JSON200 != nil && found.JSON200.CurrentExistence != nil &&
							*found.JSON200.CurrentExistence == api.CurrentExistenceExists && found.JSON200.Transition == nil
						if !onIncus {
							return fmt.Errorf("the API does not show load balancer %s on Incus with nothing pending", lbId)
						}
						return nil
					},
				),
			},
			{ResourceName: "openvicloud_load_balancer.lb", ImportState: true, ImportStateVerify: true},
			{
				Config: lbConfig(name, name+"-renamed", http+", "+dns, "openvicloud_vm.a.id, openvicloud_vm.b.id"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("openvicloud_load_balancer.lb", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_load_balancer.lb", "name", name+"-renamed"),
					resource.TestCheckResourceAttr("openvicloud_load_balancer.lb", "listener.#", "2"),
					resource.TestCheckResourceAttr("openvicloud_load_balancer.lb", "backend_vm_ids.#", "2"),
					func(s *terraform.State) error {
						lb := s.RootModule().Resources["openvicloud_load_balancer.lb"].Primary
						if lb.ID != lbId || lb.Attributes["vip"] != vip {
							return fmt.Errorf("the update replaced the load balancer: id %s vip %s, was %s %s", lb.ID, lb.Attributes["vip"], lbId, vip)
						}
						found, err := client.TitanInfrastructureEndpointsLoadBalancerGetLoadBalancerEndpointWithResponse(context.Background(), lbId)
						if err != nil {
							return err
						}
						if found.JSON200 == nil || found.JSON200.Listeners == nil || len(*found.JSON200.Listeners) != 2 || found.JSON200.Transition != nil {
							return fmt.Errorf("the API does not show two listeners with nothing pending on %s", lbId)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccLoadBalancer_ListenerTwice_IsCheckedAtPlan(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: `
resource "openvicloud_load_balancer" "lb" {
  name           = "lb"
  network_id     = "00000000-0000-4000-8000-000000000000"
  listener       = [{ protocol = "TCP", port = 80, target_port = 8080 }, { protocol = "TCP", port = 80, target_port = 9090 }]
  backend_vm_ids = ["00000000-0000-4000-8000-000000000001"]
}`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`Listener declared twice`),
		}},
	})
}
