//(C) Copyright [2020] Hewlett Packard Enterprise Development LP
//
//Licensed under the Apache License, Version 2.0 (the "License"); you may
//not use this file except in compliance with the License. You may obtain
//a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//License for the specific language governing permissions and limitations
// under the License.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	dc "github.com/bharath-b-hpe/odimra/lib-messagebus/datacommunicator"
	"github.com/bharath-b-hpe/odimra/lib-utilities/common"
	lutilconf "github.com/bharath-b-hpe/odimra/lib-utilities/config"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/config"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfphandler"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfpmessagebus"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfpmiddleware"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfpmodel"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfputilities"
	iris "github.com/kataras/iris/v12"
)

var subscriptionInfo []rfpmodel.Device

// TokenObject will contains the generated token and public key of odimra
type TokenObject struct {
	AuthToken string `json:"authToken"`
	PublicKey []byte `json:"publicKey"`
}

func main() {
	// verifying the uid of the user
	if uid := os.Geteuid(); uid == 0 {
		log.Fatalln("Plugin Service should not be run as the root user")
	}

	if err := config.SetConfiguration(); err != nil {
		log.Fatalln("error while reading from config", err)
	}

	if err := dc.SetConfiguration(config.Data.MessageBusConf.MessageQueueConfigFilePath); err != nil {
		log.Fatalf("error while trying to set messagebus configuration: %v", err)
	}

	// CreateJobQueue defines the queue which will act as an infinite buffer
	// In channel is an entry or input channel and the Out channel is an exit or output channel
	rfphandler.In, rfphandler.Out = common.CreateJobQueue()

	// RunReadWorkers will create a worker pool for doing a specific task
	// which is passed to it as Publish method after reading the data from the channel.
	go common.RunReadWorkers(rfphandler.Out, rfpmessagebus.Publish, 1)

	intializePluginStatus()
	app()
}

func app() {
	app := routers()
	go func() {
		eventsrouters()
	}()
	conf := &lutilconf.HTTPConfig{
		Certificate:   &config.Data.KeyCertConf.Certificate,
		PrivateKey:    &config.Data.KeyCertConf.PrivateKey,
		CACertificate: &config.Data.KeyCertConf.RootCACertificate,
		ServerAddress: config.Data.PluginConf.Host,
		ServerPort:    config.Data.PluginConf.Port,
	}
	pluginServer, err := conf.GetHTTPServerObj()
	if err != nil {
		log.Fatalf("fatal: error while initializing plugin server: %v", err)
	}
	app.Run(iris.Server(pluginServer))
}

func routers() *iris.Application {
	app := iris.New()
	app.WrapRouter(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		path := r.URL.Path
		if len(path) > 1 && path[len(path)-1] == '/' && path[len(path)-2] != '/' {
			path = path[:len(path)-1]
			r.RequestURI = path
			r.URL.Path = path
		}
		next(w, r)
	})

	pluginRoutes := app.Party("/ODIM/v1")
	{
		pluginRoutes.Post("/validate", rfpmiddleware.BasicAuth, rfphandler.Validate)
		pluginRoutes.Post("/Session", rfphandler.CreateSession)
		pluginRoutes.Post("/Subscriptions", rfpmiddleware.BasicAuth, rfphandler.CreateEventSubscription)
		pluginRoutes.Delete("/Subscriptions", rfpmiddleware.BasicAuth, rfphandler.DeleteEventSubscription)

		//Adding routes related to all system gets
		systems := pluginRoutes.Party("/Systems", rfpmiddleware.BasicAuth)
		systems.Get("", rfphandler.GetResource)
		systems.Get("/{id}", rfphandler.GetResource)
		systems.Get("/{id}/Storage", rfphandler.GetResource)
		systems.Get("/{id}/BootOptions", rfphandler.GetResource)
		systems.Get("/{id}/BootOptions/{rid}", rfphandler.GetResource)
		systems.Get("/{id}/Processors", rfphandler.GetResource)
		systems.Get("/{id}/LogServices", rfphandler.GetResource)
		systems.Get("/{id}/LogServices/{rid}", rfphandler.GetResource)
		systems.Get("/{id}/LogServices/{rid}/Entries", rfphandler.GetResource)
		systems.Get("/{id}/LogServices/{rid}/Entries/{rid2}", rfphandler.GetResource)
		systems.Post("/{id}/LogServices/{rid}/Actions/LogService.ClearLog", rfphandler.GetResource)
		systems.Get("/{id}/Memory", rfphandler.GetResource)
		systems.Get("/{id}/Memory/{rid}", rfphandler.GetResource)
		systems.Get("/{id}/NetworkInterfaces", rfphandler.GetResource)
		systems.Get("/{id}/MemoryDomains", rfphandler.GetResource)
		systems.Get("/{id}/EthernetInterfaces", rfphandler.GetResource)
		systems.Get("/{id}/EthernetInterfaces/{rid}", rfphandler.GetResource)
		systems.Get("/{id}/SecureBoot", rfphandler.GetResource)
		systems.Get("/{id}/EthernetInterfaces/{id2}/VLANS", rfphandler.GetResource)
		systems.Get("/{id}/EthernetInterfaces/{id2}/VLANS/{rid}", rfphandler.GetResource)
		systems.Get("/{id}/NetworkInterfaces/{rid}", rfphandler.GetResource)
		systems.Patch("/{id}", rfphandler.ChangeSettings)

		systemsAction := systems.Party("/{id}/Actions")
		systemsAction.Post("/ComputerSystem.Reset", rfphandler.ResetComputerSystem)
		systemsAction.Post("/ComputerSystem.SetDefaultBootOrder", rfphandler.SetDefaultBootOrder)

		biosParty := pluginRoutes.Party("/systems/{id}/bios")
		biosParty.Get("", rfphandler.GetResource)
		biosParty.Get("/settings", rfphandler.GetResource)
		biosParty.Patch("/settings", rfphandler.ChangeSettings)

		chassis := pluginRoutes.Party("/Chassis")
		chassis.Get("", rfphandler.GetResource)
		chassis.Get("/{id}", rfphandler.GetResource)
		chassis.Get("/{id}/NetworkAdapters", rfphandler.GetResource)

		// Chassis Power URl routes
		chassisPower := chassis.Party("/{id}/Power")
		chassisPower.Get("/", rfphandler.GetResource)
		chassisPower.Get("#PowerControl/{id1}", rfphandler.GetResource)
		chassisPower.Get("#PowerSupplies/{id1}", rfphandler.GetResource)
		chassisPower.Get("#Redundancy/{id1}", rfphandler.GetResource)

		// Chassis Thermal Url Routes
		chassisThermal := chassis.Party("/{id}/Thermal")
		chassisThermal.Get("/", rfphandler.GetResource)
		chassisThermal.Get("#Fans/{id1}", rfphandler.GetResource)
		chassisThermal.Get("#Temperatures/{id1}", rfphandler.GetResource)

		// Manager routers
		managers := pluginRoutes.Party("/Managers", rfpmiddleware.BasicAuth)
		managers.Get("", rfphandler.GetManagersCollection)
		managers.Get("/{id}", rfphandler.GetManagersInfo)
		managers.Get("/{id}/EthernetInterfaces", rfphandler.GetResource)
		managers.Get("/{id}/EthernetInterfaces/{rid}", rfphandler.GetResource)
		managers.Get("/{id}/NetworkProtocol", rfphandler.GetResource)
		managers.Get("/{id}/NetworkProtocol/{rid}", rfphandler.GetResource)
		managers.Get("/{id}/HostInterfaces", rfphandler.GetResource)
		managers.Get("/{id}/HostInterfaces/{rid}", rfphandler.GetResource)
		managers.Get("/{id}/VirtualMedia", rfphandler.GetResource)
		managers.Get("/{id}/VirtualMedia/{rid}", rfphandler.GetResource)
		managers.Get("/{id}/LogServices", rfphandler.GetResource)
		managers.Get("/{id}/LogServices/{rid}", rfphandler.GetResource)
		managers.Get("/{id}/LogServices/{rid}/Entries", rfphandler.GetResource)
		managers.Get("/{id}/LogServices/{rid}/Entries/{rid2}", rfphandler.GetResource)
		managers.Post("/{id}/LogServices/{rid}/Actions/LogService.ClearLog", rfphandler.GetResource)

		//Registries routers
		registries := pluginRoutes.Party("/Registries", rfpmiddleware.BasicAuth)
		registries.Get("", rfphandler.GetResource)
		registries.Get("/{id}", rfphandler.GetResource)

		registryStore := pluginRoutes.Party("/registrystore", rfpmiddleware.BasicAuth)
		registryStore.Get("/registries/en/{id}", rfphandler.GetResource)

		registryStoreCap := pluginRoutes.Party("/RegistryStore", rfpmiddleware.BasicAuth)
		registryStoreCap.Get("/registries/en/{id}", rfphandler.GetResource)
	}
	pluginRoutes.Get("/Status", rfphandler.GetPluginStatus)
	pluginRoutes.Post("/Startup", rfpmiddleware.BasicAuth, rfphandler.GetPluginStartup)
	return app
}

func eventsrouters() {
	app := iris.New()
	app.Post(config.Data.EventConf.DestURI, rfphandler.RedfishEvents)
	conf := &lutilconf.HTTPConfig{
		Certificate:   &config.Data.KeyCertConf.Certificate,
		PrivateKey:    &config.Data.KeyCertConf.PrivateKey,
		CACertificate: &config.Data.KeyCertConf.RootCACertificate,
		ServerAddress: config.Data.EventConf.ListenerHost,
		ServerPort:    config.Data.EventConf.ListenerPort,
	}
	evtServer, err := conf.GetHTTPServerObj()
	if err != nil {
		log.Fatalf("fatal: error while initializing event server: %v", err)
	}
	app.Run(iris.Server(evtServer))
}

// intializePluginStatus sets plugin status
func intializePluginStatus() {
	rfputilities.Status.Available = "yes"
	rfputilities.Status.Uptime = time.Now().Format(time.RFC3339)

}