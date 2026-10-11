package workspace

import (
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"golang.org/x/sys/unix"
)

func TestObservationUnchangedRequiresCtimeAndSettledObservation(t *testing.T) {
	ctime := time.Unix(1_000, 0).UnixNano()
	settled := repository.Projection{Kind: history.KindFile, Digest: history.Digest{1}, ObservedSize: 8, ObservedMtimeNS: 500, ObservedCtimeNS: ctime, ObservedInode: 7, LastScannedNS: ctime + (3 * time.Second).Nanoseconds()}
	stat := unix.Stat_t{Size: 8, Ino: 7, Mode: unix.S_IFREG | 0o600, Mtim: unix.NsecToTimespec(500), Ctim: unix.NsecToTimespec(ctime)}
	if !observationUnchanged(settled, stat) {
		t.Fatal("settled identical observation was not trusted")
	}
	changedCtime := stat
	changedCtime.Ctim = unix.NsecToTimespec(ctime + 1)
	if observationUnchanged(settled, changedCtime) {
		t.Fatal("ctime change with restored size and mtime was trusted")
	}
	racy := settled
	racy.LastScannedNS = ctime + time.Millisecond.Nanoseconds()
	if observationUnchanged(racy, stat) {
		t.Fatal("observation recorded within the timestamp window was trusted")
	}
	legacy := settled
	legacy.ObservedCtimeNS = 0
	if observationUnchanged(legacy, stat) {
		t.Fatal("observation without a recorded ctime was trusted")
	}
}
