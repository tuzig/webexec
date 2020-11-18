package main

import (
	"fmt"
	"github.com/pion/webrtc/v2"
	"sync"
)

type DCsDB struct {
	dcs map[uint16]*webrtc.DataChannel
	m   sync.RWMutex
}

func NewDCsDB() *DCsDB {
	return &DCsDB{dcs: make(map[uint16]*webrtc.DataChannel)}
}

func (dd *DCsDB) Add(d *webrtc.DataChannel) {
	id := d.ID()
	if id == nil {
		Logger.Errorf("Can not add channel %q as it's not open", d.Label())
		return
	}
	Logger.Infof("Ading data channel %d", id)
	dd.dcs[*id] = d
}

func (dd *DCsDB) Len() int {
	return len(dd.dcs)
}

func (dd *DCsDB) All() []*webrtc.DataChannel {
	dd.m.Lock()
	defer dd.m.Unlock()

	dcs := make([]*webrtc.DataChannel, 0, dd.Len())
	for _, p := range dd.dcs {
		dcs = append(dcs, p)
	}
	return dcs
}

func (dd *DCsDB) Delete(id uint16) error {
	dd.m.Lock()
	defer dd.m.Unlock()

	_, ok := dd.dcs[id]
	if !ok {
		return fmt.Errorf("data channel %#v not found", id)
	}
	Logger.Infof("Deleting data channel %d", id)
	delete(dd.dcs, id)
	return nil
}

func (dd *DCsDB) Get(id uint16) *webrtc.DataChannel {
	Logger.Infof("Deleting data channel %d", id)
	dd.m.RLock()
	defer dd.m.RUnlock()
	return dd.dcs[id]
}

func (dd *DCsDB) CloseAll() {
	for _, dc := range dd.All() {
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			dc.Close()
		}
	}
}
