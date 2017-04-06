package kafka

import (
	"github.com/funkygao/dbus/engine"
	"github.com/funkygao/dbus/pkg/model"
	conf "github.com/funkygao/jsconf"
	log "github.com/funkygao/log4go"
)

// KafkaInput is an input plugin that consumes data stream from a single specified kafka topic.
type KafkaInput struct {
	stopChan chan struct{}
}

func (this *KafkaInput) Init(config *conf.Conf) {
	this.stopChan = make(chan struct{})
}

func (this *KafkaInput) Stop(r engine.InputRunner) {
	log.Trace("[%s] stopping...", r.Name())
	close(this.stopChan)
}

func (this *KafkaInput) OnAck(pack *engine.Packet) error {
	return nil
}

func (this *KafkaInput) Run(r engine.InputRunner, h engine.PluginHelper) error {
	for {
		select {
		case <-this.stopChan:
			return nil

		case pack, ok := <-r.InChan():
			if !ok {
				log.Debug("yes sir!")
				break
			}

			pack.Payload = model.Bytes("hello world")
			r.Inject(pack)
		}
	}

	return nil
}
