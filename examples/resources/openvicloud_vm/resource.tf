data "openvicloud_templates" "here" {
  cluster_id = "singapore01"
}

resource "openvicloud_ssh_key" "me" {
  name       = "me"
  public_key = file("~/.ssh/id_ed25519.pub")
}

resource "openvicloud_vm" "web" {
  name           = "web-1"
  cluster_id     = "singapore01"
  network_id     = openvicloud_network.web.id
  template_id    = data.openvicloud_templates.here.templates[0].id
  vm_offering_id = "s"
  ssh_key_ids    = [openvicloud_ssh_key.me.id]
}
