package verifydata

import (
	"context"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/certusone/radiance/pkg/blockstore"
	"github.com/linxGnu/grocksdb"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
)

var Cmd = cobra.Command{
	Use:   "verify-data <rocksdb>",
	Short: "Verify ledger data integrity",
	Long: "Iterates through all data shreds and performs sanity checks.\n" +
		"Useful for checking the correctness of the Radiance implementation.\n" +
		"\n" +
		"Scans through the data-shreds column family with multiple threads (divide-and-conquer).",
	Args: cobra.ExactArgs(1),
}

var flags = Cmd.Flags()

var (
	flagWorkers = flags.UintP("workers", "w", uint(runtime.NumCPU()), "Number of goroutines to verify with")
	flagMaxErrs = flags.Uint32("max-errors", 100, "Abort after N errors")
	flagStatIvl = flags.Duration("stat-interval", 5*time.Second, "Stats interval")
)

// TODO add a progress bar :3

func init() {
	Cmd.Run = run
}

func run(c *cobra.Command, args []string) {
	start := time.Now()

	workers := *flagWorkers
	if workers == 0 {
		workers = uint(runtime.NumCPU())
	}

	rocksDB := args[0]
	db, err := blockstore.OpenReadOnly(rocksDB)
	if err != nil {
		klog.Exitf("Failed to open blockstore: %s", err)
	}
	defer db.Close()

	// total amount of slots
	slotLo, slotHi, ok := slotBounds(db)
	if !ok {
		klog.Exitf("Cannot find slot boundaries")
	}
	if slotLo > slotHi {
		panic("wtf: slotLo > slotHi")
	}
	total := slotHi - slotLo
	klog.Infof("Verifying %d slots", total)

	// per-worker amount of slots
	step := total / uint64(workers)
	if step == 0 {
		step = 1
	}
	cursor := slotLo
	klog.Infof("Slots per worker: %d", step)

	// stats trackers
	var numSuccess atomic.Uint64
	var numFailure atomic.Uint32

	// application lifetime
	ctx := c.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	stats := func() {
		klog.Infof("[stats] good=%d bad=%d", numSuccess.Load(), numFailure.Load())
	}

	statInterval := *flagStatIvl
	if statInterval > 0 {
		ticker := time.NewTicker(statInterval)
		group.Go(func() error {
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					stats()
				}
			}
		})
	}

	for i := uint(0); i < workers; i++ {
		// Find segment assigned to worker
		wLo := cursor
		wHi := wLo + step
		if wHi > slotHi {
			wHi = slotHi
		}
		cursor = wHi
		if wLo >= wHi {
			break
		}

		w := &worker{
			stop:        wHi,
			numSuccess:  &numSuccess,
			numFailures: &numFailure,
			maxFailures: *flagMaxErrs,
		}
		w.init(db, wLo)
		group.Go(func() error {
			return w.run(ctx)
		})
	}

	var exitCode int
	if err := group.Wait(); err != nil {
		klog.Errorf("Aborting: %s", err)
		exitCode = 1
	} else {
		klog.Info("Done!")
		exitCode = 0
	}

	stats()
	klog.Infof("Time taken: %s", time.Since(start))
	os.Exit(exitCode)
}

// slotBounds returns the lowest and highest available slots in the meta table.
func slotBounds(db *blockstore.DB) (low uint64, high uint64, ok bool) {
	iter := db.DB.NewIteratorCF(grocksdb.NewDefaultReadOptions(), db.CfMeta)
	defer iter.Close()

	iter.SeekToFirst()
	if ok = iter.Valid(); !ok {
		return
	}
	low, ok = blockstore.ParseSlotKey(iter.Key().Data())
	if !ok {
		return
	}

	iter.SeekToLast()
	if ok = iter.Valid(); !ok {
		return
	}
	high, ok = blockstore.ParseSlotKey(iter.Key().Data())
	high++
	return
}
