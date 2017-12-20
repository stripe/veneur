package pdbcycle

import (
	"bytes"
	"context"
	"github.com/boltdb/bolt"
	"github.com/signalfx/golib/boltcycle"
	"github.com/signalfx/golib/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type blob struct {
	db       *bolt.DB
	filename string
}

var prefix = "PDB_"
var suffix = ".bolt"

type syncblobs struct {
	dbs []*blob
	mu  sync.RWMutex
}

func (s *syncblobs) foreachView(fn func(*bolt.Tx) error) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.dbs) - 1; err == nil && i >= 0; i-- {
		err = s.dbs[i].db.View(fn)
	}
	return err
}

func (s *syncblobs) length() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.dbs)
}

func (s *syncblobs) get(i int) *bolt.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dbs[i].db
}

func (s *syncblobs) latest() *bolt.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dbs[len(s.dbs)-1].db
}

func (s *syncblobs) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := make([]error, 0)
	for _, db := range s.dbs {
		errs = append(errs, db.db.Close())
	}
	return errors.NewMultiErr(errs)
}

func (s *syncblobs) init(c *CyclePDB) error {
	err := s.findBlobs(c)
	if err == nil {
		err = s.purgeOldBlobs(c)
	}
	return err
}

func (s *syncblobs) findBlobs(c *CyclePDB) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := ioutil.ReadDir(c.dataDir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if strings.HasPrefix(f.Name(), prefix) && strings.HasSuffix(f.Name(), suffix) {
			blob, err := c.getBlob(filepath.Join(c.dataDir, f.Name()))
			if err != nil {
				return err
			}
			if !blob.db.IsReadOnly() {
				s.dbs = append(s.dbs, blob)
			}
		}
	}
	return nil
}

func (s *syncblobs) purgeOldBlobs(c *CyclePDB) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.dbs) > c.minNumOldBuckets {
		err = s.dbs[0].db.Close()
		if err == nil {
			err = os.Remove(s.dbs[0].filename)
			if err == nil {
				s.dbs = s.dbs[1:]
			}
		}
	}
	return err
}
func (s *syncblobs) cycleDbs(c *CyclePDB) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newFile := filepath.Join(c.dataDir, prefix+strconv.FormatInt(time.Now().UTC().UnixNano(), 10)+suffix)
	b, err := c.getBlob(newFile)

	if err == nil {
		s.dbs = append(s.dbs, b)
	}
	return err
}

// CyclePDB allows you to use a bolt.DB as a pseudo-LRU using a cycle of buckets
type CyclePDB struct {
	// dbs are the bolt databases and file names for them where we store things
	blobs *syncblobs
	// directory to find and store above files
	dataDir string
	// amount of time to wait to obtain a file lock on a boltdb
	diskTimeOut time.Duration
	// bucketTimesIn is the name of the bucket we are putting our rotating values in
	bucketTimesIn []byte
	// bucketTimesIn is the name of the bucket we are putting our rotating values in
	metaBucket []byte
	// minNumOldBuckets ensures you never delete an old bucket during a cycle if you have fewer than these number of buckets
	minNumOldBuckets int
	// Size of read moves to batch into a single transaction
	maxBatchSize int
	// Chan controls backlog of read moves
	readMovements chan readToLocation
	// How large the readMovements chan is when created
	readMovementBacklog int
	// log of errors
	asyncErrors chan<- error
	// wg controls waiting for the read movement loop
	wg sync.WaitGroup
	// stats records useful operation information for reporting back out by the user
	stats boltcycle.Stats
	// whether we should create a file on initialization or not
	initdb bool
}

var errorUnableToFindRootBucket = errors.New("unable to find root bucket")

// DBConfiguration are callbacks used as optional vardic parameters in New() to configure DB usage
type DBConfiguration func(*CyclePDB) error

// CycleLen sets the number of old buckets to keep around
func CycleLen(minNumOldBuckets int) DBConfiguration {
	return func(c *CyclePDB) error {
		c.minNumOldBuckets = minNumOldBuckets
		return nil
	}
}

// ReadMovementBacklog sets the size of the channel of read operations to rewrite
func ReadMovementBacklog(readMovementBacklog int) DBConfiguration {
	return func(c *CyclePDB) error {
		c.readMovementBacklog = readMovementBacklog
		return nil
	}
}

// AsyncErrors controls where we log async errors into.  If nil, they are silently dropped
func AsyncErrors(asyncErrors chan<- error) DBConfiguration {
	return func(c *CyclePDB) error {
		c.asyncErrors = asyncErrors
		return nil
	}
}

// BucketTimesIn is the sub bucket we put our cycled hashmap into
func BucketTimesIn(bucketName []byte) DBConfiguration {
	return func(c *CyclePDB) error {
		c.bucketTimesIn = bucketName
		return nil
	}
}

// MetaBucketName is the sub bucket we put our metadata
func MetaBucketName(bucketName []byte) DBConfiguration {
	return func(c *CyclePDB) error {
		c.metaBucket = bucketName
		return nil
	}
}

// DiskTimeOut configures how logn we wait to get a lock within boltdb
func DiskTimeOut(timeOut time.Duration) DBConfiguration {
	return func(c *CyclePDB) error {
		c.diskTimeOut = timeOut
		return nil
	}
}

// MaxBatchSize configures how logn we wait to get a lock within boltdb
func MaxBatchSize(size int) DBConfiguration {
	return func(c *CyclePDB) error {
		c.maxBatchSize = size
		return nil
	}
}

// InitDB initializes a new db
func InitDB(b bool) DBConfiguration {
	return func(c *CyclePDB) error {
		c.initdb = b
		return nil
	}
}

// New creates a CyclePDB to use a bolt database that cycles minNumOldBuckets buckets
func New(dataDir string, optionalParameters ...DBConfiguration) (ret *CyclePDB, err error) {
	ret = &CyclePDB{
		blobs:               &syncblobs{dbs: make([]*blob, 0)},
		dataDir:             dataDir,
		diskTimeOut:         time.Second,
		bucketTimesIn:       []byte("data"),
		metaBucket:          []byte("m"),
		minNumOldBuckets:    2,
		maxBatchSize:        1000,
		readMovementBacklog: 10000,
		initdb:              true,
	}

	for i, config := range optionalParameters {
		if err = config(ret); err != nil {
			return nil, errors.Annotatef(err, "Cannot execute config parameter %d", i)
		}
	}

	if err = ret.blobs.init(ret); err != nil {
		return nil, errors.Annotatef(err, "Cannot locate or open boltdbs at %s", dataDir)
	}

	if ret.initdb {
		err = ret.init()
	}
	if err == nil {
		ret.wg.Add(1)
		ret.readMovements = make(chan readToLocation, ret.readMovementBacklog)
		go ret.readMovementLoop()
	}
	return ret, err
}

// Stats returns introspection stats about the Database.  The members are considered alpha and
// subject to change or rename.
func (c *CyclePDB) Stats() boltcycle.Stats {
	ret := c.stats.AtomicClone()
	ret.SizeOfBacklogToCopy = len(c.readMovements)
	return ret
}

// Close ends the goroutine that moves read items to the latest bucket
func (c *CyclePDB) Close() error {
	close(c.readMovements)
	c.wg.Wait()
	return c.blobs.close()
}

func (c *CyclePDB) init() (err error) {
	if c.blobs.length() == 0 {
		err = c.CycleNodes()
	}
	return err
}

// VerifyBuckets ensures that the cycle of buckets have the correct names
func (c *CyclePDB) VerifyBuckets() error {
	return c.blobs.foreachView(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(c.bucketTimesIn)
		if bucket == nil {
			return errorUnableToFindRootBucket
		}
		return nil
	})
}

func (c *CyclePDB) initDB(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(c.bucketTimesIn)
		if err == nil {
			_, err = tx.CreateBucketIfNotExists(c.metaBucket)
		}
		return err
	})
}

func (c *CyclePDB) getBlob(file string) (ret *blob, err error) {

	newdb, err := bolt.Open(file, os.FileMode(0666), &bolt.Options{
		Timeout: c.diskTimeOut,
	})
	if err == nil {
		err = c.initDB(newdb)
		if err == nil {
			ret = &blob{db: newdb, filename: file}
		}
	}
	return ret, err
}

// CycleNodes deletes the first, oldest node in the primary bucket while there are >= minNumOldBuckets
// and creates a new, empty last node
func (c *CyclePDB) CycleNodes() error {
	atomic.AddInt64(&c.stats.TotalCycleCount, int64(1))
	err := c.blobs.purgeOldBlobs(c)
	if err == nil {
		err = c.blobs.cycleDbs(c)
	}
	return err
}

type readToLocation struct {
	// bucket we found the []byte key in
	dbndx int
	// Key we searched for
	key []byte
	// Value we found for the key, or nil of it wasn't found
	value []byte
	// needsCopy is true if we detected this item needs to be copied to the last bucket
	needsCopy bool
}

func (c *CyclePDB) readMovementLoop() {
	defer c.wg.Done()
	for {
		allMovements := drainAllMovements(c.readMovements, c.maxBatchSize)
		if allMovements == nil {
			return
		}
		if err := c.moveRecentReads(allMovements); err != nil {
			atomic.AddInt64(&c.stats.TotalErrorsDuringRecopy, 1)
			if c.asyncErrors != nil {
				c.asyncErrors <- err
			}
		}
	}
}

func drainAllMovements(readMovements <-chan readToLocation, maxBatchSize int) []readToLocation {
	allMovements := make([]readToLocation, 0, maxBatchSize)
	var rm readToLocation
	var ok bool
	if rm, ok = <-readMovements; !ok {
		return nil
	}
	allMovements = append(allMovements, rm)
TO:
	for len(allMovements) < maxBatchSize {
		select {
		case rm, ok := <-readMovements:
			if !ok {
				break TO
			}
			allMovements = append(allMovements, rm)
		default:
			break TO
		}
	}
	return allMovements
}

func (c *CyclePDB) indexToLocation(toread [][]byte) (res []readToLocation, err error) {
	res = make([]readToLocation, len(toread))
	needsCopy := false

	indexesToFetch := make(map[int][]byte, len(toread))
	for i, bytes := range toread {
		indexesToFetch[i] = bytes
	}

	err = c.blobs.foreachView(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(c.bucketTimesIn)
		if bucket == nil {
			return errorUnableToFindRootBucket
		}

		// We read values from the end to the start.  The last bucket is where we expect a read
		// heavy workload to have the key

		timeBucketCursor := bucket.Cursor()
		for index, searchBytes := range indexesToFetch {
			key, value := timeBucketCursor.Seek(searchBytes)
			if key == nil {
				continue
			}

			if bytes.Equal(key, searchBytes) {
				res[index].key = searchBytes
				res[index].value = make([]byte, len(value))
				// Note: The returned value is only valid for the lifetime of the transaction so
				//       we must copy it out
				copy(res[index].value, value)
				res[index].dbndx = 0 // can't do this any more
				res[index].needsCopy = needsCopy

				// We can remove this item since we don't need to search for it later
				delete(indexesToFetch, index)
			}
		}
		needsCopy = true
		return nil
	})
	return res, errors.Annotate(err, "cannot finish database view function")
}

func (c *CyclePDB) moveRecentReads(readLocations []readToLocation) error {
	dbndxToReadLocations := make(map[int][]readToLocation)
	for _, r := range readLocations {
		dbndxToReadLocations[r.dbndx] = append(dbndxToReadLocations[r.dbndx], r)
	}

	return c.blobs.latest().Update(func(tx *bolt.Tx) error {
		atomic.AddInt64(&c.stats.RecopyTransactionCount, int64(1))
		bucket := tx.Bucket(c.bucketTimesIn)
		if bucket == nil {
			return errorUnableToFindRootBucket
		}

		recopyCount := int64(0)
		asyncPutCount := int64(0)

		for _, readLocs := range dbndxToReadLocations {
			for _, rs := range readLocs {
				asyncPutCount++
				if err := bucket.Put(rs.key, rs.value); err != nil {
					return errors.Annotate(err, "cannot puts keys into the new bucket")
				}
			}
		}
		atomic.AddInt64(&c.stats.TotalItemsRecopied, recopyCount)
		atomic.AddInt64(&c.stats.TotalItemsAsyncPut, asyncPutCount)
		return nil
	})
}

//// Read bytes from the first available bucket.  Do not modify the returned bytes because
//// they are recopied to later cycle databases if needed.
func (c *CyclePDB) Read(toread [][]byte) ([][]byte, error) {
	atomic.AddInt64(&c.stats.TotalReadCount, int64(len(toread)))
	readLocations, err := c.indexToLocation(toread)
	if err != nil {
		return nil, errors.Annotatef(err, "fail to convert indexes to read location")
	}

	skips := int64(0)
	adds := int64(0)
	for _, readLocation := range readLocations {
		if readLocation.needsCopy {
			select {
			case c.readMovements <- readLocation:
				adds++
			default:
				skips++
			}
		}
	}
	if skips != 0 {
		atomic.AddInt64(&c.stats.TotalReadMovementsSkipped, skips)
	}
	if adds != 0 {
		atomic.AddInt64(&c.stats.TotalReadMovementsAdded, adds)
	}

	res := make([][]byte, len(readLocations))
	for i, rl := range readLocations {
		res[i] = rl.value
	}
	return res, nil
}

// AsyncWrite will enqueue a write into the same chan that moves reads to the last bucket.  You
// must not *ever* change the []byte given to towrite since you can't know when that []byte is
// finished being used.  Note that if the readMovements queue is backed up this operation will block
// until it has room.
func (c *CyclePDB) AsyncWrite(ctx context.Context, towrite []boltcycle.KvPair) {
	for _, w := range towrite {
		select {
		case <-ctx.Done():
			return
		case c.readMovements <- readToLocation{
			key:   w.Key,
			value: w.Value,
		}:
		}
	}
}

// Write a pair of key/value items into the cycle disk
func (c *CyclePDB) Write(towrite []boltcycle.KvPair) (err error) {
	atomic.AddInt64(&c.stats.TotalWriteCount, int64(len(towrite)))

	return c.blobs.latest().Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(c.bucketTimesIn)
		if bucket == nil {
			return errorUnableToFindRootBucket
		}

		for i := 0; err == nil && i < len(towrite); i++ {
			p := towrite[i]
			err = bucket.Put(p.Key, p.Value)
		}
		return err
	})
}

// Delete all the keys from every bucket that could have the keys.  Returns true/false for each key
// if it exists
func (c *CyclePDB) Delete(keys [][]byte) ([]bool, error) {
	atomic.AddInt64(&c.stats.TotalDeleteCount, int64(len(keys)))
	ret := make([]bool, len(keys))
	var err error

	for i := 0; i < c.blobs.length(); i++ {
		err = c.blobs.get(i).Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(c.bucketTimesIn)
			if bucket == nil {
				return errorUnableToFindRootBucket
			}
			cursor := bucket.Cursor()
			return deleteKeys(keys, cursor, ret)
		})
		if err != nil {
			return ret, err
		}
	}
	return ret, err
}

// DB exposes raw bolt db
func (c *CyclePDB) DB() *bolt.DB {
	return c.blobs.latest()
}

func deleteKeys(keys [][]byte, cursor *bolt.Cursor, ret []bool) (err error) {
	for index, key := range keys {
		k, _ := cursor.Seek(key)
		if bytes.Equal(k, key) {
			err = cursor.Delete()
			if err == nil {
				ret[index] = true
			}
		}
	}
	return err
}
