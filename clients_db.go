// this file define the type and functions that serve as the clients data base
package main

import (
	"fmt"
	"sync"

	"github.com/pion/webrtc/v3"
)

// Client ties together the dta channel, its peer and the pane
type Client struct {
	dc   *webrtc.DataChannel
	pane *Pane
	peer *Peer
	id   int
}

// ClientsDB represents a data channels data base
type ClientsDB struct {
	clients map[int]*Client
	m       sync.RWMutex
	lastID  int
}

// NewClientsDB return new data channels data base
func NewClientsDB() *ClientsDB {
	return &ClientsDB{clients: make(map[int]*Client)}
}

// Add adds a Client to the db
func (db *ClientsDB) Add(dc *webrtc.DataChannel, pane *Pane, peer *Peer) *Client {
	db.m.Lock()
	defer db.m.Unlock()
	id := db.lastID
	db.lastID++
	Logger.Infof("Adding data channel %d", id)
	c := &Client{dc, pane, peer, id}
	db.clients[id] = c
	return c
}

// Len returns how many clients are in the data base
func (db *ClientsDB) Len() int {
	return len(db.clients)
}

// All4Peer returns a slice with all the clients of a given peer
func (db *ClientsDB) All4Peer(peer *Peer) []*Client {
	db.m.Lock()
	defer db.m.Unlock()
	var r []*Client

	for _, v := range db.clients {
		if v.peer == peer {
			r = append(r, v)
		}
	}
	return r
}

// All4Pane returns
func (db *ClientsDB) All4Pane(pane *Pane) []*Client {
	db.m.Lock()
	defer db.m.Unlock()
	var r []*Client

	for _, v := range db.clients {
		if v.pane.ID == pane.ID {
			r = append(r, v)
		}
	}
	return r
}

// Delete removes a client from the database
func (db *ClientsDB) Delete(c *Client) error {
	db.m.Lock()
	defer db.m.Unlock()

	for k, v := range db.clients {
		if v.dc.ID() == c.dc.ID() && v.pane.ID == c.pane.ID {
			Logger.Infof("Deleting data channel %d", k)
			delete(db.clients, k)
			return nil
		}
	}
	return fmt.Errorf("Failed to delete as data channel not found: %v", c)
}
