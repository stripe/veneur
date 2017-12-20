package pdbcycle

import (
	"context"
	"errors"
	"github.com/boltdb/bolt"
	"github.com/signalfx/golib/boltcycle"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

func raceyReads(pdb *CyclePDB, closeChan <-chan struct{}, wg *sync.WaitGroup, t *testing.T) {
	defer wg.Done()
	for {
		select {
		case <-closeChan:
			return
		default:
			_, err := pdb.Read([][]byte{[]byte("one"), []byte("two"), []byte("three")})
			assert.NoError(t, err)
		}
	}

}

func raceyAsyncWrites(pdb *CyclePDB, closeChan <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		ctx := context.Background()
		select {
		case <-closeChan:
			return
		default:
			pdb.AsyncWrite(ctx, []boltcycle.KvPair{
				{Key: []byte("one"), Value: []byte("blargdata")},
				{Key: []byte("two"), Value: []byte("blargdata")},
				{Key: []byte("three"), Value: []byte("blargdata")},
			})
		}

	}
}

func raceyWrites(pdb *CyclePDB, closeChan <-chan struct{}, wg *sync.WaitGroup, t *testing.T) {
	defer wg.Done()
	for {
		select {
		case <-closeChan:
			return
		default:
			err := pdb.Write([]boltcycle.KvPair{
				{Key: []byte("one"), Value: []byte("blargdata")},
				{Key: []byte("two"), Value: []byte("blargdata")},
				{Key: []byte("three"), Value: []byte("blargdata")},
			})
			assert.NoError(t, err)
		}

	}
}

func raceyCycles(pdb *CyclePDB, closeChan <-chan struct{}, wg *sync.WaitGroup, t *testing.T) {
	defer wg.Done()
	for {
		select {
		case <-closeChan:
			return
		case <-time.After(time.Millisecond * 100):
			err := pdb.CycleNodes()
			assert.NoError(t, err)
		}

	}
}

func TestDataRaces(t *testing.T) {
	Convey("test me some data races", t, func() {
		dir, err := ioutil.TempDir(os.TempDir(), "pdbcycle")
		Print("Temp directory used: " + dir + " ")
		So(err, ShouldBeNil)
		myErrs := make(chan error)
		pdb, err := New(dir,
			AsyncErrors(myErrs),
			InitDB(true),
		)
		So(err, ShouldBeNil)
		wg := &sync.WaitGroup{}
		wg.Add(4)
		closeChan := make(chan struct{})
		go raceyReads(pdb, closeChan, wg, t)
		go raceyAsyncWrites(pdb, closeChan, wg)
		go raceyWrites(pdb, closeChan, wg, t)
		go raceyCycles(pdb, closeChan, wg, t)
		<-time.After(time.Second * 5)
		close(closeChan)
		wg.Wait()
	})
}

func Test(t *testing.T) {
	Convey("with a valid pdbcycle", t, func() {
		dir, err := ioutil.TempDir(os.TempDir(), "pdbcycle")
		Print("Temp directory used: " + dir + " ")
		So(err, ShouldBeNil)
		myErrs := make(chan error)
		pdb, err := New(dir,
			DiskTimeOut(time.Second),
			AsyncErrors(myErrs),
			BucketTimesIn([]byte("data")),
			MetaBucketName([]byte("m")),
			ReadMovementBacklog(1000),
			CycleLen(2),
			MaxBatchSize(3),
			InitDB(true),
		)
		So(err, ShouldBeNil)
		So(pdb, ShouldNotBeNil)
		Convey("test normal use", func() {
			Convey("test write, then read and cycle once", func() {
				err = pdb.Write([]boltcycle.KvPair{
					{Key: []byte("first"), Value: []byte("firstdata")},
					{Key: []byte("firsttwo"), Value: []byte("firsttwodata")},
				})
				So(err, ShouldBeNil)
				data, err := pdb.Read([][]byte{[]byte("first")})
				So(err, ShouldBeNil)
				So(len(data), ShouldEqual, 1)
				So(string(data[0]), ShouldEqual, "firstdata")
				So(pdb.CycleNodes(), ShouldBeNil)
				infos, err := ioutil.ReadDir(dir)
				So(err, ShouldBeNil)
				So(len(infos), ShouldEqual, 2)
				Convey("same, now 3 files", func() {
					err = pdb.Write([]boltcycle.KvPair{{Key: []byte("second"), Value: []byte("seconddata")}})
					So(err, ShouldBeNil)
					So(pdb.CycleNodes(), ShouldBeNil)
					infos, err = ioutil.ReadDir(dir)
					So(err, ShouldBeNil)
					So(len(infos), ShouldEqual, 3)
					Convey("read some data from first file so it gets copied", func() {
						data, err := pdb.Read([][]byte{[]byte("firsttwo")})
						So(err, ShouldBeNil)
						So(len(data), ShouldEqual, 1)
						So(string(data[0]), ShouldEqual, "firsttwodata")
						Convey("first file was deleted", func() {
							err = pdb.Write([]boltcycle.KvPair{{Key: []byte("third"), Value: []byte("thirddata")}})
							So(err, ShouldBeNil)
							So(pdb.CycleNodes(), ShouldBeNil)
							infos, err = ioutil.ReadDir(dir)
							So(err, ShouldBeNil)
							So(len(infos), ShouldEqual, 3)
							Convey("can only get data that was accessed, not the non accessed data", func() {
								for {
									data, err := pdb.Read([][]byte{[]byte("first"), []byte("firsttwo")})
									So(err, ShouldBeNil)
									So(len(data), ShouldEqual, 2)
									if string(data[0]) == "" && string(data[1]) == "firsttwodata" {
										break
									}
									runtime.Gosched()
								}
								Convey("close and reopen", func() {
									So(pdb.Close(), ShouldBeNil)
									pdb, err = New(dir,
										DiskTimeOut(time.Second),
										AsyncErrors(myErrs),
										BucketTimesIn([]byte("data")),
										ReadMovementBacklog(10000),
										CycleLen(2))
									So(err, ShouldBeNil)
									So(pdb, ShouldNotBeNil)
								})
								Convey("verify", func() {
									So(pdb.VerifyBuckets(), ShouldBeNil)
								})
								Convey("db", func() {
									So(pdb.DB(), ShouldNotBeNil)
								})
							})
						})
					})
				})
			})
		})
		Convey("delete some keys", func() {
			err = pdb.Write([]boltcycle.KvPair{{Key: []byte("blarg"), Value: []byte("blargdata")}})
			rr, err := pdb.Read([][]byte{[]byte("blarg")})
			So(err, ShouldBeNil)
			So(string(rr[0]), ShouldEqual, "blargdata")
			answers, err := pdb.Delete([][]byte{[]byte("blarg"), []byte("doesn't exist")})
			So(err, ShouldBeNil)
			So(answers[0], ShouldBeTrue)
			rr, err = pdb.Read([][]byte{[]byte("blarg")})
			So(err, ShouldBeNil)
			So(string(rr[0]), ShouldEqual, "")

		})
		Convey("test AsyncWrite", func() {
			ctx, cancel := context.WithCancel(context.Background())
			pdb.AsyncWrite(ctx, []boltcycle.KvPair{
				{Key: []byte("blarg1"), Value: []byte("blargdata")},
				{Key: []byte("blarg2"), Value: []byte("blargdata")},
				{Key: []byte("blarg3"), Value: []byte("blargdata")},
				{Key: []byte("blarg4"), Value: []byte("blargdata")},
				{Key: []byte("blarg5"), Value: []byte("blargdata")},
				{Key: []byte("blarg6"), Value: []byte("blargdata")},
				{Key: []byte("blarg7"), Value: []byte("blargdata")},
				{Key: []byte("blarg8"), Value: []byte("blargdata")},
				{Key: []byte("blarg9"), Value: []byte("blargdata")},
				{Key: []byte("blarg0"), Value: []byte("blargdata")},
			})
			for {
				rr, err := pdb.Read([][]byte{[]byte("blarg1")})
				runtime.Gosched()
				So(err, ShouldBeNil)
				if string(rr[0]) == "blargdata" {
					break
				}
				runtime.Gosched()
			}
			cancel()
			pdb.AsyncWrite(ctx, []boltcycle.KvPair{
				{Key: []byte("blarg1"), Value: []byte("blargdata")},
				{Key: []byte("blarg2"), Value: []byte("blargdata")},
				{Key: []byte("blarg3"), Value: []byte("blargdata")},
				{Key: []byte("blarg4"), Value: []byte("blargdata")},
				{Key: []byte("blarg5"), Value: []byte("blargdata")},
				{Key: []byte("blarg6"), Value: []byte("blargdata")},
				{Key: []byte("blarg7"), Value: []byte("blargdata")},
				{Key: []byte("blarg8"), Value: []byte("blargdata")},
				{Key: []byte("blarg9"), Value: []byte("blargdata")},
				{Key: []byte("blarg0"), Value: []byte("blargdata")},
			})
			for pdb.Stats().TotalReadMovementsAdded != 0 && pdb.Stats().RecopyTransactionCount != 1 {
				runtime.Gosched()
			}
		})
		Reset(func() {
			err := pdb.Close()
			So(err, ShouldBeNil)
		})
	})
}

func TestErr(t *testing.T) {
	Convey("test some error conditions", t, func() {
		Convey("test bad directory", func() {
			_, err := New("!@#$$%")
			So(err, ShouldNotBeNil)
		})
		Convey("test bad dbconfig", func() {
			_, err := New("", func(*CyclePDB) error {
				return errors.New("nope")
			})
			So(err, ShouldNotBeNil)
		})
		Convey("bad findDbs", func() {
			dir, err := ioutil.TempDir(os.TempDir(), "pdbcycle")
			So(err, ShouldBeNil)
			err = ioutil.WriteFile(filepath.Join(dir, prefix+strconv.FormatInt(time.Now().UTC().UnixNano(), 10)+suffix), []byte("blarg"), 0644)
			So(err, ShouldBeNil)
			_, err = New(dir)
			So(err, ShouldNotBeNil)
		})
		Convey("no root bucket", func() {
			dir, err := ioutil.TempDir(os.TempDir(), "pdbcycle")
			So(err, ShouldBeNil)
			myErrs := make(chan error)
			pdb, err := New(dir, AsyncErrors(myErrs))
			So(err, ShouldBeNil)

			err = pdb.blobs.latest().Update(func(tx *bolt.Tx) error {
				return tx.DeleteBucket(pdb.bucketTimesIn)
			})
			So(err, ShouldBeNil)
			Convey("async write fails", func() {
				pdb.AsyncWrite(context.Background(), []boltcycle.KvPair{{Key: []byte("blarg1"), Value: []byte("blargdata")}})
				for pdb.Stats().TotalReadMovementsAdded != 0 && pdb.Stats().RecopyTransactionCount != 1 && pdb.Stats().TotalErrorsDuringRecopy != 1 {
					runtime.Gosched()
				}
				err = <-myErrs
				So(err, ShouldNotBeNil)
			})
			Convey("read fails", func() {
				_, err := pdb.Read([][]byte{})
				So(err, ShouldNotBeNil)
			})
			Convey("write fails", func() {
				err := pdb.Write([]boltcycle.KvPair{})
				So(err, ShouldNotBeNil)
			})
			Convey("delete fails", func() {
				_, err := pdb.Delete([][]byte{[]byte("blarg"), []byte("doesn't exist")})
				So(err, ShouldNotBeNil)
			})
			Convey("verify fails", func() {
				err := pdb.VerifyBuckets()
				So(err, ShouldNotBeNil)
			})
		})
		Convey("test failed puts into new bucket", func() {
			dir, err := ioutil.TempDir(os.TempDir(), "pdbcycle")
			So(err, ShouldBeNil)
			myErrs := make(chan error, 10)
			pdb, err := New(dir, AsyncErrors(myErrs))
			So(err, ShouldBeNil)
			pdb.AsyncWrite(context.Background(), []boltcycle.KvPair{{Key: []byte(""), Value: []byte("blargdata")}})
			time.Sleep(time.Second)
		})
		Convey("test skips", func() {
			dir, err := ioutil.TempDir(os.TempDir(), "pdbcycle")
			So(err, ShouldBeNil)
			pdb, err := New(dir, ReadMovementBacklog(0))
			So(err, ShouldBeNil)
			So(pdb.Write([]boltcycle.KvPair{
				{Key: []byte("blarg1"), Value: []byte("blargdata")},
				{Key: []byte("blarg2"), Value: []byte("blargdata")},
				{Key: []byte("blarg3"), Value: []byte("blargdata")},
				{Key: []byte("blarg4"), Value: []byte("blargdata")},
				{Key: []byte("blarg5"), Value: []byte("blargdata")},
			}), ShouldBeNil)
			for pdb.Stats().TotalReadMovementsSkipped == 0 {
				So(pdb.CycleNodes(), ShouldBeNil)
				_, err = pdb.Read([][]byte{
					[]byte("blarg1"),
					[]byte("blarg2"),
					[]byte("blarg3"),
					[]byte("blarg4"),
					[]byte("blarg5"),
				})
				So(err, ShouldBeNil)
			}
		})
	})
}
