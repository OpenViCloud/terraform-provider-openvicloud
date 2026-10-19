resource "openvicloud_load_balancer" "web" {
  name           = "web"
  network_id     = openvicloud_network.web.id
  listener       = [{ protocol = "TCP", port = 80, target_port = 8080 }]
  backend_vm_ids = [openvicloud_vm.web.id]
}
