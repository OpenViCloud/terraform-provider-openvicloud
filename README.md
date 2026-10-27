# Terraform provider for OpenViCloud

Manage the resources of your OpenViCloud tenant from Terraform or OpenTofu, with an API token you create on the
OpenViCloud portal (API tokens page).

> Published to the [Terraform Registry](https://registry.terraform.io/providers/openvicloud/openvicloud/latest). The OpenTofu
> registry has not accepted it yet; until it does, OpenTofu users can build it locally (below).

## What it manages today

| Kind | Name |
|---|---|
| Data source | `openvicloud_zones`, `openvicloud_templates`, `openvicloud_vm_offerings`, `openvicloud_volume_offerings` |
| Resource | `openvicloud_network`, `openvicloud_firewall_rule`, `openvicloud_ssh_key`, `openvicloud_vm`, `openvicloud_volume`, `openvicloud_volume_attachment`, `openvicloud_load_balancer` |

```hcl
terraform {
  required_providers {
    openvicloud = { source = "openvicloud/openvicloud" }
  }
}

provider "openvicloud" {} # reads OPENVICLOUD_TOKEN, and OPENVICLOUD_API_URL when set

resource "openvicloud_network" "web" {
  name    = "web"
  zone_id = "singapore01"
}

resource "openvicloud_firewall_rule" "ssh" {
  network_id = openvicloud_network.web.id
  direction  = "Ingress"
  protocol   = "TCP"
  start_port = 22
  end_port   = 22
  cidr       = "0.0.0.0/0"
}

data "openvicloud_templates" "here" { zone_id = "singapore01" }

resource "openvicloud_ssh_key" "me" {
  name       = "me"
  public_key = file("~/.ssh/id_ed25519.pub")
}

resource "openvicloud_vm" "web" {
  name           = "web-1"
  zone_id        = "singapore01"
  network_id     = openvicloud_network.web.id
  template_id    = data.openvicloud_templates.here.templates[0].id
  vm_offering_id = "s"
  ssh_key_ids    = [openvicloud_ssh_key.me.id]
}

resource "openvicloud_volume" "data" {
  name               = "data"
  zone_id            = "singapore01"
  volume_offering_id = "standard"
  size_gb            = 20
}

resource "openvicloud_volume_attachment" "data" {
  volume_id = openvicloud_volume.data.id
  vm_id     = openvicloud_vm.web.id
}
```

Creates and deletes wait until OpenViCloud has finished them on the cloud, which can take a couple of minutes; a VM
is created when it is Running or Ready.

Terraform declares what exists, not whether a VM is on: stop and start a VM with the `openvicloud` CLI or the
portal, and Terraform leaves it as it is. Only a VM's name changes in place; any other change replaces the VM.

A load balancer (on a network with `enable_public_ipv6 = true`) changes its name, listeners and backend VMs in place
and keeps its VIP. OpenViCloud refuses to delete a VM a load balancer still sends to, so to replace such a VM, first
remove it from `backend_vm_ids` and apply, then make the change that replaces it.

```hcl
resource "openvicloud_load_balancer" "web" {
  name           = "web"
  network_id     = openvicloud_network.web.id
  listener       = [{ protocol = "TCP", port = 80, target_port = 8080 }]
  backend_vm_ids = [openvicloud_vm.web.id]
}
```

## Develop

- `go test ./...`: unit tests.
- `TF_ACC=1 go test -run TestAcc -timeout 30m .`: acceptance tests against a real OpenViCloud API; set
  `OPENVICLOUD_API_URL` and `OPENVICLOUD_TOKEN` (and `OPENVICLOUD_TEST_ZONE`, default `local-mac`). They create and
  destroy resources named `tfacc-<random>`.
- `scripts/sync-spec.sh <commit>`: copy the API contract from a commit of OpenViCloud/titan-control-plane and regenerate the client;
  `api/SOURCE` records the commit.
- `go tool tfplugindocs generate --provider-name openvicloud`: regenerate the registry pages in `docs/` from the
  schema descriptions and `examples/`; CI fails when they are out of date.
- Release: a tag `vX.Y.Z` runs `.github/workflows/release.yml`, which waits for approval in the GitHub environment
  `release` (its reviewer approves), then builds the zips with goreleaser and signs `SHA256SUMS` with the OpenViCloud
  release key (fingerprint `BB74B8ECF2BAA2321296E3BA62009A5A7C779F46`). Only admins can create, move or delete `v*`
  tags.
- Use a local build: `go build -o <dir>/terraform-provider-openvicloud .` and a CLI configuration with
  `provider_installation { dev_overrides { "openvicloud/openvicloud" = "<dir>" } direct {} }`.

## License

[MPL-2.0](LICENSE)
