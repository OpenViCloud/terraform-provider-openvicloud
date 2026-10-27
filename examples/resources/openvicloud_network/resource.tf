resource "openvicloud_network" "web" {
  name               = "web"
  cluster_id            = "singapore01"
  enable_public_ipv6 = true
}
