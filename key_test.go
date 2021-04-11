package main

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	initTest(t)
	k := KeyType{}
	privKey, err := k.generate()
	require.Nil(t, err)
	privKey2, err := k.generate()
	require.NotEqualValues(t, privKey, privKey2)
}
func TestGetCerts(t *testing.T) {
	initTest(t)
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	cs, err := GetCerts()
	require.Nil(t, err)
	require.Equal(t, 1, len(cs))
}
func TestCertsCache(t *testing.T) {
	initTest(t)
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	cs1, err := GetCerts()
	require.Nil(t, err)
	// test to ensure we're not hitting the disk - mock ioutil.ReadFile
	cs2, err := GetCerts()
	require.Nil(t, err)
	Logger.Infof("%v\n%v", cs1, cs2)
	require.True(t, cs1[0].Equals(cs2[0]))
}
func TestKeyConsistency(t *testing.T) {
	initTest(t)
	f, err := ioutil.TempFile("", "private.key")
	require.Nil(t, err)
	f.Close()
	cs1, err := GetCerts()
	require.Nil(t, err)
	cs2, err := GetCerts()
	Logger.Infof("%v\n%v", cs1, cs2)
	require.Nil(t, err)
	//	require.True(t, cs1[0].Equals(cs2[0]))
	fps1, err := cs1[0].GetFingerprints()
	require.Nil(t, err)
	fps2, err := cs2[0].GetFingerprints()
	require.Nil(t, err)
	require.EqualValues(t, fps1[0], fps2[0])
}
