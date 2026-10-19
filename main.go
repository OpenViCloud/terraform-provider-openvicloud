// The OpenViCloud Terraform provider. main is the only place that knows every package: it lists the data sources
// and resources the provider offers.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/OpenViCloud/terraform-provider-openvicloud/datasource"
	"github.com/OpenViCloud/terraform-provider-openvicloud/provider"
	"github.com/OpenViCloud/terraform-provider-openvicloud/resource/firewallrule"
	"github.com/OpenViCloud/terraform-provider-openvicloud/resource/network"
	tfdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// version is set by the release build (-ldflags "-X main.version=…").
var version = "dev"

func DataSources() []func() tfdatasource.DataSource {
	return []func() tfdatasource.DataSource{
		datasource.NewZones, datasource.NewTemplates, datasource.NewVmOfferings, datasource.NewVolumeOfferings,
	}
}

func Resources() []func() resource.Resource {
	return []func() resource.Resource{network.New, firewallrule.New}
}

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version, DataSources(), Resources()), providerserver.ServeOpts{
		Address: "registry.terraform.io/openvicloud/openvicloud",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
