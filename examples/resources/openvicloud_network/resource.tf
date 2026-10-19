resource "openvicloud_network" "web" {
  name               = "web"
  zone_id            = "singapore01"
  enable_public_ipv6 = true
}
