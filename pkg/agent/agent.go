package agent

import (
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/petethepig/pyroscope/pkg/agent/csock"
	"github.com/petethepig/pyroscope/pkg/agent/upstream"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/util/id"
)

type Agent struct {
	cfg            *config.Config
	upstream       *upstream.Upstream
	activeProfiles map[int]*profileSession
	id             id.ID
}

func New(cfg *config.Config) *Agent {
	return &Agent{
		cfg:            cfg,
		upstream:       upstream.New(cfg),
		activeProfiles: make(map[int]*profileSession),
	}
}

func (a *Agent) Start() {
	sockPath := a.cfg.Agent.UNIXSocketPath
	cs, err := csock.NewUnixCSock(sockPath, a.callback)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(sockPath)

	a.upstream.Start()

	log.WithField("addr", cs.CanonicalAddr()).Info("Starting control socket")
	cs.Start()
}

func (a *Agent) callback(req *csock.Request) *csock.Response {
	log.Debug("callback:", req)
	switch req.Command {
	case "start":
		s := newSession(req.SpyName, req.Pid)
		profileID := int(a.id.Next())
		a.activeProfiles[profileID] = s

		err := s.start()
		if err != nil {
			log.Debugf("failed to start spy session: %q, pid: %d", req.SpyName, req.Pid)
		}

		return &csock.Response{ProfileID: profileID}
	case "stop":
		if sess, ok := a.activeProfiles[req.ProfileID]; ok {
			t := sess.stop()
			// TODO: these should be passed from integrations
			metadata := map[string]string{
				"name":  "testapp.cpu{}",
				"from":  strconv.Itoa(int(sess.startTime.Unix())),
				"until": strconv.Itoa(int(sess.stopTime.Unix())),
			}
			a.upstream.Upload(metadata, t)
		} else {
			log.Debugf("failed to find spy session: %d", req.ProfileID)
		}
		return &csock.Response{}
	default:
		return &csock.Response{}
	}
}