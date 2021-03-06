package engine

import (
	"io"
	"net/http"
	"strconv"

	"github.com/funkygao/dbus/pkg/cluster"
	log "github.com/funkygao/log4go"
)

const (
	maxRPCBodyLen = 128 << 10
)

// POST /v1/rebalance?epoch={controller_epoch}
func (e *Engine) doLocalRebalanceV1(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > maxRPCBodyLen {
		log.Warn("too large RPC request body: %d", r.ContentLength)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	body := make([]byte, r.ContentLength)
	_, err := io.ReadFull(r.Body, body)
	if err != nil {
		log.Error("%v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	epoch, _ := strconv.Atoi(r.FormValue("epoch"))
	e.Lock()
	if epoch < e.epoch {
		e.Unlock()
		http.Error(w, "leader moved", http.StatusNotAcceptable)
		return
	}
	e.epoch = epoch // remember the leader epoch
	e.Unlock()

	phase := r.FormValue("phase")
	if len(phase) == 0 {
		http.Error(w, "empty phase", http.StatusBadRequest)
		return
	}

	resources := cluster.UnmarshalRPCResources(body)
	log.Debug("local dispatching %d resources: %v", len(resources), resources)

	// merge resources by input plugin name
	inputResourcesMap := make(map[string][]cluster.Resource) // inputName:resources
	for _, res := range resources {
		if _, present := inputResourcesMap[res.InputPlugin]; !present {
			inputResourcesMap[res.InputPlugin] = []cluster.Resource{res}
		} else {
			inputResourcesMap[res.InputPlugin] = append(inputResourcesMap[res.InputPlugin], res)
		}
	}

	switch phase {
	case "1":
		foundInputToStop := false
		keepRunningN := 0
		e.irmMu.Lock()
		for inputName := range e.irm {
			if _, present := inputResourcesMap[inputName]; !present {
				// will close this input
				if ir, ok := e.InputRunners[inputName]; ok {
					log.Trace("phase1: stopping Input[%s]", inputName)
					ir.feedResources(nil)
					foundInputToStop = true
				}
			} else {
				// this input plugin continues just like nothing happened
				keepRunningN++
			}
		}
		e.irm = inputResourcesMap
		e.irmMu.Unlock()

		if !foundInputToStop {
			log.Trace("phase1: no one to stop, %d Input keep running", keepRunningN)
		}

	case "2":
		// dispatch decision to input plugins
		for inputName, rs := range inputResourcesMap {
			if ir, ok := e.InputRunners[inputName]; ok {
				log.Trace("phase2: feed %s %+v", inputName, rs)
				ir.feedResources(rs)
			} else {
				// should never happen
				// if it happens, must be human operation fault
				log.Critical("phase2: feed %s renounced %+v", inputName, rs)

				if err := e.controller.RenounceResources(rs); err != nil {
					log.Error("%+v %s", rs, err)
				}
			}
		}

	default:
		// should never happen
		log.Critical("phase=%s?", phase)
		http.Error(w, "invalid phase", http.StatusBadRequest)
	}

}
