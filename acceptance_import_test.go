package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/resource/network"
	"github.com/OpenViCloud/terraform-provider-openvicloud/resource/vm"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var testClock = wait.Clock{Now: time.Now, Sleep: time.Sleep}

// madeOutsideTerraform is a network, an SSH key and a VM with that key, created straight on the API, as a customer
// who starts using Terraform has them.
type madeOutsideTerraform struct{ networkId, keyId, vmId, templateId string }

func makeOutsideTerraform(t *testing.T, client *api.ClientWithResponses, name, publicKey, userData string) madeOutsideTerraform {
	ctx := context.Background()
	var made madeOutsideTerraform
	z := cluster()
	templates, err := client.TitanInfrastructureEndpointsCatalogListTemplatesEndpointWithResponse(ctx,
		&api.TitanInfrastructureEndpointsCatalogListTemplatesEndpointParams{ClusterId: &z})
	if err != nil || templates.JSON200 == nil || len(*templates.JSON200) == 0 {
		t.Fatalf("no template in cluster %s: %v", z, err)
	}
	made.templateId = *(*templates.JSON200)[0].Id

	created, err := client.TitanInfrastructureEndpointsNetworkCreateNetworkEndpointWithResponse(ctx,
		&api.TitanInfrastructureEndpointsNetworkCreateNetworkEndpointParams{IdempotencyKey: api.NewIdempotencyKey()},
		api.CreateNetworkCommand{Name: name, ClusterId: z})
	if err != nil || created.JSON202 == nil {
		t.Fatalf("creating the network: %v %s", err, created.Body)
	}
	made.networkId = *created.JSON202.NetworkId
	err = wait.Until(ctx, testClock, 10*time.Minute, 5*time.Second, func(ctx context.Context) (bool, error) {
		n, err := client.TitanInfrastructureEndpointsNetworkGetNetworkEndpointWithResponse(ctx, made.networkId)
		if err != nil || n.JSON200 == nil {
			return false, err
		}
		return network.Created(*n.JSON200)
	})
	if err != nil {
		t.Fatalf("waiting for the network: %v", err)
	}

	key, err := client.TitanInfrastructureEndpointsSshKeyAddSshKeyEndpointWithResponse(ctx,
		&api.TitanInfrastructureEndpointsSshKeyAddSshKeyEndpointParams{IdempotencyKey: api.NewIdempotencyKey()},
		api.AddSshKeyCommand{Name: &name, PublicKey: &publicKey})
	if err != nil || key.JSON201 == nil {
		t.Fatalf("adding the key: %v %s", err, key.Body)
	}
	made.keyId = *key.JSON201.Id

	keys := []string{made.keyId}
	machine, err := client.TitanInfrastructureEndpointsVmCreateVmEndpointWithResponse(ctx,
		&api.TitanInfrastructureEndpointsVmCreateVmEndpointParams{IdempotencyKey: api.NewIdempotencyKey()},
		api.CreateVmCommand{Name: name, ClusterId: z, NetworkId: made.networkId, TemplateId: made.templateId,
			VmOfferingId: "s", SshKeyIds: &keys, UserData: &userData})
	if err != nil || machine.JSON202 == nil {
		t.Fatalf("creating the VM: %v %s", err, machine.Body)
	}
	made.vmId = *machine.JSON202.VmId
	err = wait.Until(ctx, testClock, 10*time.Minute, 5*time.Second, func(ctx context.Context) (bool, error) {
		v, err := client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, made.vmId)
		if err != nil || v.JSON200 == nil {
			return false, err
		}
		return vm.Created(*v.JSON200)
	})
	if err != nil {
		t.Fatalf("waiting for the VM: %v", err)
	}
	return made
}

// cleanUpOutsideTerraform deletes what makeOutsideTerraform made when the test ends before Terraform owns it, so a
// failed run does not hold the tenant's network quota.
func cleanUpOutsideTerraform(t *testing.T, client *api.ClientWithResponses, made *madeOutsideTerraform) {
	ctx := context.Background()
	gone := func(check func(ctx context.Context) (bool, error)) {
		if err := wait.Until(ctx, testClock, 10*time.Minute, 5*time.Second, check); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
	if made.vmId != "" {
		gone(func(ctx context.Context) (bool, error) {
			v, err := client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, made.vmId)
			return err == nil && (v.JSON200 == nil || vm.Deletable(*v.JSON200)), nil
		})
		_, _ = client.TitanInfrastructureEndpointsVmDeleteVmEndpointWithResponse(ctx, made.vmId)
		gone(func(ctx context.Context) (bool, error) {
			v, err := client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, made.vmId)
			if err != nil {
				return false, nil
			}
			return vm.Gone(v.StatusCode(), v.JSON200)
		})
	}
	if made.keyId != "" {
		_, _ = client.TitanInfrastructureEndpointsSshKeyDeleteSshKeyEndpointWithResponse(ctx, made.keyId)
	}
	if made.networkId != "" {
		gone(func(ctx context.Context) (bool, error) {
			n, err := client.TitanInfrastructureEndpointsNetworkGetNetworkEndpointWithResponse(ctx, made.networkId)
			return err == nil && network.Deletable(n.JSON200), nil
		})
		_, _ = client.TitanInfrastructureEndpointsNetworkDeleteNetworkEndpointWithResponse(ctx, made.networkId)
		gone(func(ctx context.Context) (bool, error) {
			n, err := client.TitanInfrastructureEndpointsNetworkGetNetworkEndpointWithResponse(ctx, made.networkId)
			if err != nil {
				return false, nil
			}
			return network.Gone(n.StatusCode(), n.JSON200)
		})
	}
}

// The API never returns an SSH key itself nor the keys a VM was created with, so importing them into a configuration
// that names them must change them in place: a replacement would destroy the customer's VM.
func TestAccImportedVmAndKey_AreUpdatedInPlace_NotReplaced(t *testing.T) {
	// Its setup calls the API before resource.Test, which is what skips the other acceptance tests without TF_ACC.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test: set TF_ACC=1")
	}
	preCheck(t)
	client := oracle(t)
	name := "tfacc-" + acctest.RandString(8)
	publicKey := newPublicKey(t)
	userData := "#cloud-config\nhostname: " + name + "\n"

	var made madeOutsideTerraform
	owned := false
	t.Cleanup(func() {
		if !owned {
			cleanUpOutsideTerraform(t, client, &made)
		}
	})
	made = makeOutsideTerraform(t, client, name, publicKey, userData)

	config := fmt.Sprintf(`
import {
  to = openvicloud_network.n
  id = %[1]q
}
import {
  to = openvicloud_ssh_key.k
  id = %[2]q
}
import {
  to = openvicloud_vm.v
  id = %[3]q
}

resource "openvicloud_network" "n" {
  name    = %[5]q
  cluster_id = %[6]q
}

resource "openvicloud_ssh_key" "k" {
  name       = %[5]q
  public_key = %[7]q
}

resource "openvicloud_vm" "v" {
  name           = %[5]q
  cluster_id        = %[6]q
  network_id     = openvicloud_network.n.id
  template_id    = %[4]q
  vm_offering_id = "s"
  ssh_key_ids    = [openvicloud_ssh_key.k.id]
  user_data      = %[8]q
}
`, made.networkId, made.keyId, made.vmId, made.templateId, name, cluster(), publicKey, userData)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             func(_ *terraform.State) error { return leftovers(client, name) },
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("openvicloud_network.n", plancheck.ResourceActionNoop),
					plancheck.ExpectResourceAction("openvicloud_ssh_key.k", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("openvicloud_vm.v", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeTestCheckFunc(
					func(_ *terraform.State) error { owned = true; return nil },
					resource.TestCheckResourceAttr("openvicloud_vm.v", "id", made.vmId),
					resource.TestCheckResourceAttr("openvicloud_vm.v", "ssh_key_ids.#", "1"),
					resource.TestCheckResourceAttr("openvicloud_ssh_key.k", "id", made.keyId),
					func(_ *terraform.State) error {
						v, err := client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(context.Background(), made.vmId)
						if err != nil {
							return err
						}
						if v.StatusCode() != http.StatusOK || v.JSON200.CurrentExistence == nil || *v.JSON200.CurrentExistence != api.CurrentExistenceExists {
							return errors.New("the imported VM is not alive on the API after apply")
						}
						return nil
					},
				),
			},
		},
	})
}
