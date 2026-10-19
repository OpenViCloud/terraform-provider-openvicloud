terraform {
  required_providers {
    openvicloud = {
      source = "openvicloud/openvicloud"
    }
  }
}

# Reads OPENVICLOUD_TOKEN (an API token created on the portal) and, when set, OPENVICLOUD_API_URL.
provider "openvicloud" {}
