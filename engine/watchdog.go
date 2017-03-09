package engine

import (
	"time"

	log "github.com/funkygao/log4go"
)

func (e *Engine) runWatchdog(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	var inputPoolSize, filterPoolSize int

	for range t.C { // FIXME need to be more frequent
		inputPoolSize = len(e.inputRecycleChan)
		filterPoolSize = len(e.filterRecycleChan)
		if inputPoolSize == 0 || filterPoolSize == 0 {
			log.Warn("Recycle pool reservation: [input]%d [filter]%d", inputPoolSize, filterPoolSize)
		}
	}
}