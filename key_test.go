package main

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

func TestGetCerts(t *testing.T) {
	initTest(t)
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	k := KeyType{Path: f.Name()}
	cs, err := k.GetCerts()
	require.Nil(t, err)
	require.Equal(t, 1, len(cs))
}
func TestCertsCache(t *testing.T) {
	initTest(t)
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	k1 := KeyType{Path: f.Name()}
	cs1, err := k1.GetCerts()
	require.Nil(t, err)
	cs2, err := k1.GetCerts()
	require.Nil(t, err)
	Logger.Infof("%v\n%v", cs1, cs2)
	require.True(t, cs1[0].Equals(cs2[0]))
}
func TestKeyConsistency(t *testing.T) {
	initTest(t)
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	k1 := KeyType{Path: f.Name()}
	cs1, err := k1.GetCerts()
	require.Nil(t, err)
	k2 := KeyType{Path: f.Name()}
	cs2, err := k2.GetCerts()
	Logger.Infof("%v\n%v", cs1, cs2)
	require.Nil(t, err)
	//	require.True(t, cs1[0].Equals(cs2[0]))
	fps1, err := cs1[0].GetFingerprints()
	require.Nil(t, err)
	fps2, err := cs2[0].GetFingerprints()
	require.Nil(t, err)
	require.Equal(t, fps1[0], fps2[0])
}
