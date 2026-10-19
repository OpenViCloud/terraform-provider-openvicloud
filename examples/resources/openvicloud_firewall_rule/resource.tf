resource "openvicloud_firewall_rule" "ssh" {
  network_id = openvicloud_network.web.id
  direction  = "Ingress"
  protocol   = "TCP"
  start_port = 22
  end_port   = 22
  cidr       = "0.0.0.0/0"
}
