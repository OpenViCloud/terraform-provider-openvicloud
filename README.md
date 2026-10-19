# Terraform provider for OpenViCloud

Manage the resources of your OpenViCloud tenant from Terraform or OpenTofu, with an API token you create on the
OpenViCloud portal (API tokens page).

> Not published to the Terraform or OpenTofu registry yet. Until the first release, build it locally (below).

## What it manages today

| Kind | Name |
|---|---|
| Data source | `openvicloud_zones`, `openvicloud_templates`, `openvicloud_vm_offerings`, `openvicloud_volume_offerings` |
| Resource | `openvicloud_network`, `openvicloud_firewall_rule` |

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
```

Creates and deletes wait until OpenViCloud has finished them on the cloud, which can take a couple of minutes.

## Develop

- `go test ./...`: unit tests.
- `TF_ACC=1 go test -run TestAcc -timeout 30m .`: acceptance tests against a real OpenViCloud API; set
  `OPENVICLOUD_API_URL` and `OPENVICLOUD_TOKEN` (and `OPENVICLOUD_TEST_ZONE`, default `local-mac`). They create and
  destroy resources named `tfacc-<random>`.
- `scripts/sync-spec.sh <commit>`: copy the API contract from a commit of OpenViCloud/titan and regenerate the client;
  `api/SOURCE` records the commit.
- Use a local build: `go build -o <dir>/terraform-provider-openvicloud .` and a CLI configuration with
  `provider_installation { dev_overrides { "openvicloud/openvicloud" = "<dir>" } direct {} }`.

## License

[MPL-2.0](LICENSE)
