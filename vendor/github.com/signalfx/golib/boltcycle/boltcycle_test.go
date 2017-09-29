package boltcycle

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/signalfx/golib/errors"

	"bytes"

	"github.com/boltdb/bolt"
	"github.com/signalfx/golib/log"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type testSetup struct {
	filename             string
	cdb                  *CycleDB
	cycleLen             int
	readMovementsBacklog int
	log                  bytes.Buffer
	t                    *testing.T
	failedDelete         bool
	asyncErrors          chan error
	readOnly             bool
}

func (t *testSetup) Errorf(format string, args ...interface{}) {
	t.t.Errorf(format, args...)
}

func (t *testSetup) FailNow() {
	buf := make([]byte, 1024)
	runtime.Stack(buf, false)
	t.t.Errorf("%s\n", string(buf))
	t.t.FailNow()
}

func (t *testSetup) Close() error {
	require.NoError(t, t.cdb.Close())
	require.NoError(t, t.cdb.db.Close())
	require.NoError(t, os.Remove(t.filename))
	return nil
}

func setupCdb(t *testing.T, cycleLen int) testSetup {
	f, err := ioutil.TempFile("", "TestDiskCache")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(f.Name()))
	ret := testSetup{
		filename:             f.Name(),
		cycleLen:             cycleLen,
		t:                    t,
		readMovementsBacklog: 10,
		asyncErrors:          make(chan error),
	}
	ret.canOpen()
	return ret
}

func (t *testSetup) canOpen() {
	db, err := bolt.Open(t.filename, os.FileMode(0666), &bolt.Options{
		ReadOnly: t.readOnly,
		Timeout:  time.Second,
	})
	require.NoError(t, err)

	args := []DBConfiguration{CycleLen(t.cycleLen), ReadMovementBacklog(t.readMovementsBacklog), AsyncErrors(t.asyncErrors)}
	if t.failedDelete {
		args = append(args, func(c *CycleDB) error {
			c.cursorDelete = func(*bolt.Cursor) error {
				return errors.New("nope")
			}
			return nil
		})
	}

	t.cdb, err = New(db, args...)
	require.NoError(t, err)
	require.NoError(t, t.cdb.VerifyCompressed())
}

func (t *testSetup) isEmpty(key string) {
	r, err := t.cdb.Read([][]byte{[]byte(key)})
	require.NoError(t, err)
	require.Equal(t, [][]byte{nil}, r)
}

func (t *testSetup) canWrite(key string, value string) {
	require.NoError(t, t.cdb.Write([]KvPair{{[]byte(key), []byte(value)}}))
}

func (t *testSetup) canDelete(key string, exists bool) {
	e, err := t.cdb.Delete([][]byte{[]byte(key)})
	require.NoError(t, err)
	require.Equal(t, e[0], exists)
}

func (t *testSetup) equals(key string, value string) {
	r, err := t.cdb.Read([][]byte{[]byte(key)})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte(value)}, r)
}

func (t *testSetup) canCycle() {
	require.NoError(t, t.cdb.CycleNodes())
}

func (t *testSetup) canClose() {
	require.NoError(t, t.cdb.db.Close())
}

func (t *testSetup) isVerified() {
	require.NoError(t, t.cdb.VerifyBuckets())
}

func (t *testSetup) isCompressed() {
	require.NoError(t, t.cdb.VerifyCompressed())
}

func TestCursorHeap(t *testing.T) {
	c := cursorHeap([]stringCursor{})
	c.Push(stringCursor{head: "a"})
	c.Push(stringCursor{head: "b"})
	require.True(t, c.Less(0, 1))
}

func TestErrUnexpectedNonBucket(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	require.NoError(t, testRun.cdb.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(testRun.cdb.bucketTimesIn).Put([]byte("hi"), []byte("world"))
	}))
	require.Equal(t, errUnexpectedNonBucket, errors.Tail(testRun.cdb.VerifyBuckets()))
	_, err := testRun.cdb.Delete([][]byte{[]byte("hello")})
	require.Equal(t, errUnexpectedNonBucket, err)

	_, err = testRun.cdb.Read([][]byte{[]byte("hello")})
	require.Equal(t, errUnexpectedNonBucket, errors.Tail(err))
}

func TestErrUnexpectedBucketBytes(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	require.NoError(t, testRun.cdb.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.Bucket(testRun.cdb.bucketTimesIn).CreateBucket([]byte("_"))
		return err
	}))
	require.Equal(t, errUnexpectedBucketBytes, testRun.cdb.VerifyBuckets())
}

func TestVerifyCompressed(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.canWrite("hello", "world")
	testRun.canWrite("hello2", "world")
	testRun.canCycle()
	testRun.canWrite("hello", "world")

	require.Equal(t, errOrderingWrong, testRun.cdb.VerifyCompressed())

	e1 := errors.New("nope")
	createHeapFunc = func(*bolt.Bucket) (cursorHeap, error) {
		return nil, e1
	}

	require.Equal(t, e1, errors.Tail(testRun.cdb.VerifyCompressed()))

	createHeapFunc = createHeap
}

func TestErrNoLastBucket(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.cdb.bucketTimesIn = []byte("empty_bucket")
	require.NoError(t, testRun.cdb.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(testRun.cdb.bucketTimesIn)
		log.IfErr(log.Panic, b.Put([]byte("hello"), []byte("invalid_setup")))
		return err
	}))
	require.Equal(t, errNoLastBucket, testRun.cdb.moveRecentReads(nil))
	require.Equal(t, errNoLastBucket, testRun.cdb.Write([]KvPair{}))

	require.NoError(t, testRun.cdb.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(testRun.cdb.bucketTimesIn)
		return b.Delete([]byte("hello"))
	}))
	require.Equal(t, errNoLastBucket, testRun.cdb.moveRecentReads(nil))
	require.Equal(t, errNoLastBucket, testRun.cdb.Write([]KvPair{}))
}

func TestMoveRecentReads(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.canWrite("hello", "world")
	testRun.canCycle()
	testRun.canClose()
	testRun.readOnly = true
	testRun.canOpen()
	testRun.equals("hello", "world")
}

func TestAsyncWriteEventuallyHappens(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.cdb.AsyncWrite(context.Background(), []KvPair{{Key: []byte("hello"), Value: []byte("world")}})
	for testRun.cdb.Stats().TotalItemsAsyncPut == 0 {
		runtime.Gosched()
	}
	// No assert needed.  Testing that write eventually happens
}

func TestAsyncWriteBadDelete(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.canClose()
	testRun.failedDelete = true
	testRun.canOpen()
	testRun.canWrite("hello", "world")
	testRun.canCycle()
	testRun.equals("hello", "world")
	e := <-testRun.asyncErrors
	require.Equal(t, "nope", e.Error())
}

func TestBoltCycleRead(t *testing.T) {
	Convey("when setup to fail", t, func() {
		testRun := setupCdb(t, 5)
		log.IfErr(log.Panic, testRun.Close())
		testRun.readMovementsBacklog = 0
		testRun.failedDelete = true
		testRun.canOpen()
		testRun.canWrite("hello", "world")
		So(testRun.cdb.stats.TotalReadMovementsSkipped, ShouldEqual, 0)
		testRun.canCycle()
		Convey("and channel is full because error is blocking", func() {
			testRun.equals("hello", "world")
			Convey("future reads should not block", func() {
				// This should not block
				testRun.equals("hello", "world")
				So(testRun.cdb.stats.TotalReadMovementsSkipped, ShouldBeGreaterThan, 0)
			})
		})
		Reset(func() {
			go func() {
				<-testRun.asyncErrors
			}()

			log.IfErr(log.Panic, testRun.Close())
			close(testRun.asyncErrors)
		})
	})
}

func TestAsyncWrite(t *testing.T) {
	Convey("when setup", t, func() {
		c := CycleDB{}
		Convey("and channel is full", func() {
			c.readMovements = make(chan readToLocation)
			Convey("Async write should timeout", func() {
				ctx, _ := context.WithTimeout(context.Background(), time.Millisecond)
				c.AsyncWrite(ctx, []KvPair{{}})
			})
		})
	})
}

func TestAsyncWriteBadValue(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.cdb.AsyncWrite(context.Background(), []KvPair{{Key: []byte(""), Value: []byte("world")}})
	require.Equal(t, bolt.ErrKeyRequired, errors.Tail(<-testRun.asyncErrors))
}

func TestErrOnWrite(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	require.Error(t, testRun.cdb.Write([]KvPair{{}}))
}

func TestErrOnDelete(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.canWrite("hello", "world")
	testRun.canCycle()

	err := testRun.cdb.db.View(func(tx *bolt.Tx) error {
		var bucketName [8]byte
		binary.BigEndian.PutUint64(bucketName[:], 1)
		cur := tx.Bucket(testRun.cdb.bucketTimesIn).Bucket(bucketName[:]).Cursor()
		return deleteKeys([][]byte{[]byte("hello")}, cur, []bool{false})
	})
	require.Equal(t, bolt.ErrTxNotWritable, errors.Tail(err))
}

func TestErrUnableToFindRootBucket(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.cdb.bucketTimesIn = []byte("not_here")
	require.Equal(t, errUnableToFindRootBucket, testRun.cdb.VerifyBuckets())
	require.Equal(t, errUnableToFindRootBucket, testRun.cdb.VerifyCompressed())
	require.Equal(t, errUnableToFindRootBucket, testRun.cdb.CycleNodes())
	require.Equal(t, errUnableToFindRootBucket, testRun.cdb.moveRecentReads(nil))

	_, err := testRun.cdb.Read([][]byte{[]byte("hello")})
	require.Equal(t, errUnableToFindRootBucket, errors.Tail(err))

	err = testRun.cdb.Write([]KvPair{{[]byte("hello"), []byte("world")}})
	require.Equal(t, errUnableToFindRootBucket, errors.Tail(err))

	_, err = testRun.cdb.Delete([][]byte{[]byte("hello")})
	require.Equal(t, errUnableToFindRootBucket, errors.Tail(err))
}

func TestDatabaseInit(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.canClose()
	testRun.canOpen()
	_, err := New(testRun.cdb.db, BucketTimesIn([]byte{}))
	require.Error(t, err)
}

func TestCycleNodes(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.cdb.minNumOldBuckets = 0
	require.NoError(t, testRun.cdb.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(testRun.cdb.bucketTimesIn).Put([]byte("hi"), []byte("world"))
	}))
	require.Equal(t, bolt.ErrIncompatibleValue, errors.Tail(testRun.cdb.CycleNodes()))
}

func TestDatabaseCycle(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()
	testRun.isVerified()

	testRun.isEmpty("hello")
	testRun.canCycle()
	testRun.canWrite("hello", "world")
	testRun.isCompressed()
	testRun.equals("hello", "world")

	startingMoveCount := testRun.cdb.Stats().TotalItemsRecopied
	testRun.canCycle()
	testRun.equals("hello", "world")
	// Give "hello" time to copy to the last bucket
	for startingMoveCount == testRun.cdb.Stats().TotalItemsRecopied {
		runtime.Gosched()
	}

	for i := 0; i < testRun.cycleLen; i++ {
		testRun.canCycle()
		testRun.isCompressed()
	}

	begin := testRun.cdb.Stats().TotalItemsRecopied
	testRun.equals("hello", "world")
	for testRun.cdb.Stats().TotalItemsRecopied == begin {
		runtime.Gosched()
	}

	for testRun.cdb.Stats().SizeOfBacklogToCopy > 0 {
		runtime.Gosched()
	}

	for i := 0; i < testRun.cycleLen+1; i++ {
		testRun.canCycle()
		testRun.isCompressed()
	}

	testRun.isEmpty("hello")
	testRun.isCompressed()
}

func TestReadDelete(t *testing.T) {
	testRun := setupCdb(t, 5)
	defer func() {
		log.IfErr(log.Panic, testRun.Close())
	}()

	testRun.isEmpty("hello")
	testRun.canDelete("hello", false)
	testRun.isEmpty("hello")

	testRun.canWrite("hello", "world")
	testRun.equals("hello", "world")

	testRun.canDelete("hello", true)
	testRun.canDelete("hello", false)
	testRun.isEmpty("hello")

	testRun.canWrite("hello", "world")
	testRun.canCycle()
	testRun.canDelete("hello", true)
	testRun.isEmpty("hello")
}

func TestBadInit(t *testing.T) {
	expected := errors.New("nope")
	_, err := New(nil, func(*CycleDB) error { return expected })
	require.Equal(t, expected, errors.Tail(err))
}

func TestDrainAllMovements(t *testing.T) {
	{
		readMovements := make(chan readToLocation)
		maxBatchSize := 10
		close(readMovements)
		n := drainAllMovements(readMovements, maxBatchSize)
		require.Nil(t, n)
	}
	{
		readMovements := make(chan readToLocation, 3)
		maxBatchSize := 10
		readMovements <- readToLocation{bucket: 101}
		close(readMovements)
		n := drainAllMovements(readMovements, maxBatchSize)
		require.Equal(t, []readToLocation{{bucket: 101}}, n)
	}
	{
		readMovements := make(chan readToLocation, 3)
		maxBatchSize := 2
		readMovements <- readToLocation{bucket: 101}
		readMovements <- readToLocation{bucket: 102}
		close(readMovements)
		n := drainAllMovements(readMovements, maxBatchSize)
		require.Equal(t, []readToLocation{{bucket: 101}, {bucket: 102}}, n)
	}
}
