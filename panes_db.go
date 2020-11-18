package main

import (
	"fmt"
	"sync"
)

type PanesDB struct {
	panes  map[int]*Pane
	m      sync.RWMutex
	nextID int
}

func NewPanesDB() *PanesDB {
	return &PanesDB{panes: make(map[int]*Pane)}
}

func (pd *PanesDB) Add(p *Pane) {
	pd.m.Lock()
	defer pd.m.Unlock()

	pd.nextID++
	p.ID = pd.nextID
	pd.panes[p.ID] = p
}

func (pd *PanesDB) All() []*Pane {
	pd.m.Lock()
	defer pd.m.Unlock()

	panes := make([]*Pane, 0, len(pd.panes))
	for _, p := range pd.panes {
		panes = append(panes, p)
	}
	return panes

}

func (pd *PanesDB) Delete(id int) error {
	pd.m.Lock()
	defer pd.m.Unlock()

	_, ok := pd.panes[id]
	if !ok {
		return fmt.Errorf("pane %#v not found", id)
	}
	delete(pd.panes, id)
	return nil
}

func (pd *PanesDB) Get(id int) *Pane {
	pd.m.RLock()
	defer pd.m.RUnlock()

	return pd.panes[id]
}
