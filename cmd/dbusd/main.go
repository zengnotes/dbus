package main

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/funkygao/dbus"
	"github.com/funkygao/dbus/engine"
	"github.com/funkygao/gafka/ctx"
	"github.com/funkygao/gafka/diagnostics/agent"
	"github.com/funkygao/log4go"

	// bootstrap plugins
	_ "github.com/funkygao/dbus/plugins/filter"
	_ "github.com/funkygao/dbus/plugins/input"
	_ "github.com/funkygao/dbus/plugins/output"
)

func init() {
	parseFlags()

	if options.showversion {
		showVersionAndExit()
	}

	setupLogging()

	ctx.LoadFromHome()
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
			debug.PrintStack()
		}
	}()

	globals := engine.DefaultGlobals()
	globals.Debug = options.debug
	globals.RouterTrack = options.routerTrack
	globals.InputRecyclePoolSize = options.inputPoolSize
	globals.FilterRecyclePoolSize = options.filterPoolSize
	globals.HubChanSize = options.hubPoolSize
	globals.PluginChanSize = options.pluginPoolSize
	globals.ClusterEnabled = options.clusterEnable

	if !options.validateConf && len(options.visualizeFile) == 0 {
		// daemon mode
		log4go.Info("dbus[%s@%s] starting", dbus.Revision, dbus.Version)

		agent.HttpAddr = options.pprofAddr
		log4go.Info("pprof agent ready on %s", agent.Start())
		go func() {
			log4go.Error("%s", <-agent.Errors)
		}()
	}

	t0 := time.Now()
	var err error
	for {
		e := engine.New(globals).LoadConfig(options.configPath)

		if options.visualizeFile != "" {
			e.ExportDiagram(options.visualizeFile)
			return
		}

		if options.validateConf {
			fmt.Println("ok")
			return
		}

		if err = e.ServeForever(); err != nil {
			// e,g. SIGTERM received
			log4go.Info("%v", err)
			break
		}

		globals.Stopping = false
	}

	log4go.Info("dbus[%s@%s] %s, bye!", dbus.Revision, dbus.Version, time.Since(t0))
	log4go.Close()
}
