// A script to test kafka async and ack mechanism.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Shopify/sarama"
	"github.com/funkygao/dbus/pkg/kafka"
	"github.com/funkygao/gafka/ctx"
	"github.com/funkygao/gafka/diagnostics/agent"
	"github.com/funkygao/gafka/zk"
	"github.com/funkygao/golib/color"
	"github.com/funkygao/golib/gofmt"
	"github.com/funkygao/golib/signal"
	"github.com/funkygao/golib/sync2"
	"github.com/funkygao/log4go"
)

var (
	zone, cluster, topic string
	ack                  string
	syncMode             bool
	maxErrs              int64
	msgSize              int
	messages             int
	sleep                time.Duration
	slient               bool

	_ sarama.Encoder = &payload{}

	inChan = make(chan sarama.Encoder)
)

type payload struct {
	i int64
	s string

	b   []byte
	err error
}

func (p *payload) Encode() ([]byte, error) {
	p.ensureEncoded()
	return p.b, p.err
}

func (p *payload) Length() int {
	p.ensureEncoded()
	return len(p.b)
}

func (p *payload) String() string {
	return fmt.Sprintf("%8d", p.i)
}

func (p *payload) ensureEncoded() {
	if len(p.b) == 0 {
		p.b, p.err = sarama.StringEncoder(fmt.Sprintf("{%09d} %s", p.i, p.s)).Encode()
	}
}

func init() {
	ctx.LoadFromHome()

	flag.StringVar(&zone, "z", "prod", "zone")
	flag.StringVar(&cluster, "c", "", "cluster")
	flag.StringVar(&topic, "t", "", "topic")
	flag.StringVar(&ack, "ack", "local", "local|none|all")
	flag.BoolVar(&syncMode, "sync", false, "sync mode")
	flag.Int64Var(&maxErrs, "e", 10, "max errors before quit")
	flag.IntVar(&msgSize, "sz", 1024*10, "message size")
	flag.IntVar(&messages, "n", 1024, "flush messages")
	flag.BoolVar(&slient, "s", true, "silent mode")
	flag.DurationVar(&sleep, "sleep", 0, "sleep between producing messages")
	flag.Parse()

	if len(zone) == 0 || len(cluster) == 0 || len(topic) == 0 {
		panic("invalid flag")
	}

	if !slient {
		sarama.Logger = log.New(os.Stdout, color.Magenta("[Sarama]"), log.LstdFlags|log.Lshortfile)
	}
	log4go.SetLevel(log4go.TRACE)

	agent.Start()
}

func main() {
	cf := kafka.DefaultConfig()
	cf.Sarama.Producer.Flush.Messages = messages
	if syncMode {
		cf.SyncMode()
	}
	switch ack {
	case "none":
		cf.Ack(sarama.NoResponse)
	case "local":
		cf.Ack(sarama.WaitForLocal)
	case "all":
		cf.Ack(sarama.WaitForAll)
	default:
		panic("invalid: " + ack)
	}

	p := kafka.NewProducer("tester", zk.NewZkZone(zk.DefaultConfig(zone, ctx.ZoneZkAddrs(zone))).NewCluster(cluster).BrokerList(), cf)

	var (
		sent, sentOk sync2.AtomicInt64
	)

	p.SetErrorHandler(func(err *sarama.ProducerError) {
		v := err.Msg.Value.(*payload)
		log.Println(color.Red("no -> %d %s", v.i, err))
	})
	p.SetSuccessHandler(func(msg *sarama.ProducerMessage) {
		v := msg.Value.(*payload)
		log.Println(color.Green("ok -> %d", v.i))
		sentOk.Add(1)
	})

	if err := p.Start(); err != nil {
		panic(err)
	}

	closed := make(chan struct{})
	var once sync.Once
	signal.RegisterHandler(func(sig os.Signal) {
		log.Printf("got signal %s", sig)

		once.Do(func() {
			close(closed)
		})
	}, syscall.SIGINT)

	go func() {
		for {
			time.Sleep(time.Second * 5)
			log.Println(gofmt.Comma(sent.Get()), "->", gofmt.Comma(sentOk.Get()))
		}
	}()

	go func() {
		var i int64
		for {
			log4go.Info(color.Blue("->> %d", i))
			inChan <- &payload{i: i, s: strings.Repeat("X", msgSize)}
			i++
		}
	}()

	for {
		select {
		case <-closed:
			goto BYE

		case msg := <-inChan:
			if err := p.Send(&sarama.ProducerMessage{Topic: topic, Value: msg}); err != nil {
				log.Println(err)
				goto BYE
			}

			sent.Add(1)
			if sleep > 0 {
				time.Sleep(sleep)
			}

		}
	}

BYE:
	log.Printf("%d/%d, closed with %v", sentOk.Get(), sent.Get(), p.Close())

}