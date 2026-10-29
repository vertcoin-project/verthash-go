package verthash

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var verthashTestInput []byte
var verthashTestOutput []byte
var testDatafile = filepath.Join(os.TempDir(), "verthash_verthash_go_testsuite.dat")

func TestMain(m *testing.M) {
	fmt.Println("Verthash test vectors require generation of Verthash file, this will be slow!")
	os.Remove(testDatafile)
	if err := MakeVerthashDatafile(testDatafile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	verthashTestInput, _ = hex.DecodeString("000000203a297b4b7685170d7644b43e5a6056234cc2414edde454a87580e1967d14c1078c13ea916117b0608732f3f65c2e03b81322efc0a62bcee77d8a9371261970a58a5a715da80e031b02560ad8")
	verthashTestOutput, _ = hex.DecodeString("E0F6C10B4A38F35A6CDCC26D32A7ED8C3BFC5D827A9BC72647AFA324B70D0463")
	code := m.Run()
	os.Remove(testDatafile)
	os.Exit(code)
}

func TestCreation(t *testing.T) {
	ok, err := VerifyVerthashDatafile(testDatafile)
	if err != nil || !ok {
		t.Error(err)
	}
}

func TestHashDisk(t *testing.T) {
	vh, err := NewVerthash(testDatafile, false)
	if err != nil {
		t.Error(err)
	}
	defer vh.Close()

	h, err := vh.SumVerthash(verthashTestInput)
	if !bytes.Equal(h[:], verthashTestOutput) {
		t.Errorf("Verthash output invalid: %x vs %x", h, verthashTestOutput)
	}

}

func TestHashMemory(t *testing.T) {
	vh, err := NewVerthash(testDatafile, true)
	if err != nil {
		t.Error(err)
	}
	defer vh.Close()

	h, err := vh.SumVerthash(verthashTestInput)
	if !bytes.Equal(h[:], verthashTestOutput) {
		//os.Remove("/tmp/verthash.dat")
		t.Errorf("Verthash output invalid: %x vs %x", h, verthashTestOutput)
	}
}

// One Verthash is shared by goroutines hashing at the same time, from the file
// and from the mapping.
func TestHashConcurrent(t *testing.T) {
	for _, keepInRam := range []bool{false, true} {
		vh, err := NewVerthash(testDatafile, keepInRam)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 20; i++ {
					h, err := vh.SumVerthash(verthashTestInput)
					if err != nil || !bytes.Equal(h[:], verthashTestOutput) {
						t.Errorf("keepInRam=%v: got %x, %v", keepInRam, h, err)
						return
					}
				}
			}()
		}
		wg.Wait()
		vh.Close()
	}
}
