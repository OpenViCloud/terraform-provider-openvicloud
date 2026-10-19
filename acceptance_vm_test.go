package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/resource/sshkey"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"golang.org/x/crypto/ssh"
)

func newPublicKey(t *testing.T) string {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

func vmConfig(name, vmName, publicKey string) string {
	return fmt.Sprintf(`
data "openvicloud_templates" "here" { zone_id = %[4]q }

resource "openvicloud_network" "n" {
  name    = %[1]q
  zone_id = %[4]q
}

resource "openvicloud_ssh_key" "k" {
  name       = %[1]q
  public_key = %[3]q
}

resource "openvicloud_vm" "v" {
  name           = %[2]q
  zone_id        = %[4]q
  network_id     = openvicloud_network.n.id
  template_id    = data.openvicloud_templates.here.templates[0].id
  vm_offering_id = "s"
  ssh_key_ids    = [openvicloud_ssh_key.k.id]
  user_data      = "#cloud-config\nhostname: %[1]s\n"
}

resource "openvicloud_volume" "d" {
  name               = %[1]q
  zone_id            = %[4]q
  volume_offering_id = "standard"
  size_gb            = 1
}

resource "openvicloud_volume_attachment" "a" {
  volume_id = openvicloud_volume.d.id
  vm_id     = openvicloud_vm.v.id
}
`, name, vmName, publicKey, zone())
}

// leftovers finds what a test named name left on the API: anything not Deleted whose name starts with it. It goes by
// name, not by id, so it still works when the test failed before it read the ids.
func leftovers(client *api.ClientWithResponses, name string) error {
	ctx := context.Background()
	var left []string
	alive := func(kind string, n *string, existence *api.CurrentExistence) {
		if n != nil && strings.HasPrefix(*n, name) && (existence == nil || *existence != api.CurrentExistenceDeleted) {
			left = append(left, kind+" "+*n)
		}
	}
	vms, err := client.TitanInfrastructureEndpointsVmListVmsEndpointWithResponse(ctx)
	if err != nil {
		return err
	}
	for _, v := range *vms.JSON200 {
		alive("VM", v.Name, v.CurrentExistence)
	}
	volumes, err := client.TitanInfrastructureEndpointsVolumeListVolumesEndpointWithResponse(ctx)
	if err != nil {
		return err
	}
	for _, v := range *volumes.JSON200 {
		alive("volume", v.Name, v.CurrentExistence)
	}
	keys, err := client.TitanInfrastructureEndpointsSshKeyListSshKeysEndpointWithResponse(ctx)
	if err != nil {
		return err
	}
	for _, k := range *keys.JSON200 {
		alive("SSH key", k.Name, nil)
	}
	balancers, err := client.TitanInfrastructureEndpointsLoadBalancerListLoadBalancersEndpointWithResponse(ctx)
	if err != nil {
		return err
	}
	for _, lb := range *balancers.JSON200 {
		alive("load balancer", lb.Name, lb.CurrentExistence)
	}
	networks, err := client.TitanInfrastructureEndpointsNetworkListNetworksEndpointWithResponse(ctx)
	if err != nil {
		return err
	}
	for _, n := range *networks.JSON200 {
		alive("network", n.Name, n.CurrentExistence)
	}
	if len(left) > 0 {
		return fmt.Errorf("left after destroy: %s", strings.Join(left, ", "))
	}
	return nil
}

func listedVolume(client *api.ClientWithResponses, id string) (*api.VolumeQueryResult, error) {
	response, err := client.TitanInfrastructureEndpointsVolumeListVolumesEndpointWithResponse(context.Background())
	if err != nil {
		return nil, err
	}
	for _, v := range *response.JSON200 {
		if v.Id != nil && *v.Id == id {
			return &v, nil
		}
	}
	return nil, nil
}

func TestAccVmWithKeyAndVolume_CreateImportRenameDestroy(t *testing.T) {
	client := oracle(t)
	name := "tfacc-" + acctest.RandString(8)
	publicKey := newPublicKey(t)
	wantFingerprint, err := sshkey.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	var vmId, volumeId string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             func(s *terraform.State) error { return leftovers(client, name) },
		Steps: []resource.TestStep{
			{
				Config: vmConfig(name, name, publicKey),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_ssh_key.k", "fingerprint", wantFingerprint),
					resource.TestCheckResourceAttr("openvicloud_vm.v", "name", name),
					resource.TestCheckResourceAttr("openvicloud_vm.v", "ssh_key_ids.#", "1"),
					resource.TestCheckResourceAttrPair("openvicloud_volume_attachment.a", "vm_id", "openvicloud_vm.v", "id"),
					func(s *terraform.State) error {
						resources := s.RootModule().Resources
						vmId = resources["openvicloud_vm.v"].Primary.ID
						volumeId = resources["openvicloud_volume.d"].Primary.ID

						vm, err := client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(context.Background(), vmId)
						if err != nil {
							return err
						}
						state := vm.JSON200.CurrentState
						if state == nil || (*state != api.VmStateRunning && *state != api.VmStateReady) {
							return fmt.Errorf("the API shows the VM in state %v after apply, want Running or Ready", state)
						}
						volume, err := listedVolume(client, volumeId)
						if err != nil {
							return err
						}
						if volume == nil || volume.CurrentAttachedVmId == nil || *volume.CurrentAttachedVmId != vmId {
							return fmt.Errorf("the API does not show volume %s attached to VM %s", volumeId, vmId)
						}
						return nil
					},
				),
			},
			{
				ResourceName: "openvicloud_ssh_key.k", ImportState: true, ImportStateVerify: true,
				// The API never returns the key itself, only its fingerprint.
				ImportStateVerifyIgnore: []string{"public_key"},
			},
			{
				ResourceName: "openvicloud_vm.v", ImportState: true, ImportStateVerify: true,
				// The API never returns the keys a VM was created with.
				ImportStateVerifyIgnore: []string{"ssh_key_ids"},
			},
			{ResourceName: "openvicloud_volume.d", ImportState: true, ImportStateVerify: true},
			{ResourceName: "openvicloud_volume_attachment.a", ImportState: true, ImportStateVerify: true},
			{
				Config: vmConfig(name, name+"-renamed", publicKey),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("openvicloud_vm.v", "name", name+"-renamed"),
					func(s *terraform.State) error {
						if id := s.RootModule().Resources["openvicloud_vm.v"].Primary.ID; id != vmId {
							return fmt.Errorf("rename replaced the VM: id %s, was %s", id, vmId)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccVmAndKey_BadValuesAreCheckedAtPlan(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "openvicloud_ssh_key" "k" {
  name       = "k"
  public_key = "not a key"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Not a public key`),
			},
			{
				Config: fmt.Sprintf(`
resource "openvicloud_vm" "v" {
  name           = "-starts-with-hyphen"
  zone_id        = %q
  network_id     = "00000000-0000-4000-8000-000000000000"
  template_id    = "00000000-0000-4000-8000-000000000000"
  vm_offering_id = "s"
}`, zone()),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`starting with a letter or digit`),
			},
		},
	})
}
