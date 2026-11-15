package imap

import (
	"errors"
	"slices"
)

// ErrInvalidUID is returned when an appended UID is zero or not greater than
// the highest UID already present in the view.
var ErrInvalidUID = errors.New("imap: invalid UID")

// View is a compact per-session mailbox view. UIDs are append-only, and the
// view maps them to IMAP sequence numbers after expunges.
//
// The view stores one slot per appended UID. A Fenwick tree records which slots
// are occupied. This gives O(log n) sequence number lookups and O(log n)
// removals while keeping the code small.
type View struct {
	uidValidity uint32
	highestUID  uint32
	count       uint32
	slots       []uint32
	tree        []int
	pos         map[uint32]int
}

// NewView creates a view for the given UIDVALIDITY and UIDs. The UIDs are
// sorted and duplicate values are removed. Zero UIDs are ignored.
func NewView(uidValidity uint32, uids []uint32) *View {
	v := &View{}
	v.Reset(uidValidity, uids)

	return v
}

// Reset replaces the view contents.
func (v *View) Reset(uidValidity uint32, uids []uint32) {
	sorted := make([]uint32, 0, len(uids))

	for _, uid := range uids {
		if uid != 0 {
			sorted = append(sorted, uid)
		}
	}

	slices.Sort(sorted)

	v.uidValidity = uidValidity
	v.slots = v.slots[:0]
	v.pos = make(map[uint32]int, len(sorted))
	v.count = 0
	v.highestUID = 0

	for _, uid := range sorted {
		if v.highestUID != 0 && uid <= v.highestUID {
			continue
		}

		v.pos[uid] = len(v.slots)
		v.slots = append(v.slots, uid)
		v.highestUID = uid
		v.count++
	}

	v.buildTree()
}

//nolint:funcorder
func (v *View) buildTree() {
	v.tree = make([]int, len(v.slots)+1)

	for i := range v.slots {
		v.tree[i+1] = 1
	}

	for i := 1; i <= len(v.slots); i++ {
		if next := i + (i & -i); next <= len(v.slots) {
			v.tree[next] += v.tree[i]
		}
	}
}

//nolint:funcorder
func (v *View) add(index, delta int) {
	for i := index + 1; i < len(v.tree); i += i & -i {
		v.tree[i] += delta
	}
}

// prefix returns the number of occupied slots in [0, end).
//
//nolint:funcorder
func (v *View) prefix(end int) int {
	sum := 0
	for i := end; i > 0; i -= i & -i {
		sum += v.tree[i]
	}

	return sum
}

// kth returns the slot index of the k-th occupied slot, where k starts at 1.
// It returns -1 when k is out of range.
//
//nolint:funcorder
func (v *View) kth(k uint32) int {
	if k == 0 || k > v.count {
		return -1
	}

	index := 0

	bit := 1
	for bit <= len(v.slots) {
		bit <<= 1
	}

	for bit >>= 1; bit > 0; bit >>= 1 {
		next := index + bit
		if next <= len(v.slots) && v.tree[next] < int(k) {
			index = next
			k -= uint32(v.tree[next]) //nolint:gosec
		}
	}

	return index
}

// Append adds a new UID to the end of the view. The UID must be greater than
// every UID already in the view.
func (v *View) Append(uid uint32) error {
	if uid == 0 || uid <= v.highestUID {
		return ErrInvalidUID
	}

	index := len(v.slots)
	v.slots = append(v.slots, uid)
	v.tree = append(v.tree, 0)

	// The new tree node covers a range that did not exist in the old tree.
	// Initialize it from the existing slots before adding the new leaf.
	node := len(v.slots)
	lowbit := node & -node
	v.tree[node] = v.prefix(node-1) - v.prefix(node-lowbit)
	v.add(index, 1)
	v.pos[uid] = index
	v.highestUID = uid
	v.count++

	return nil
}

// Remove removes a UID from the view. It returns the sequence number the UID
// had before removal and whether the UID was present.
func (v *View) Remove(uid uint32) (uint32, bool) {
	index, ok := v.pos[uid]
	if !ok {
		return 0, false
	}

	seq := uint32(v.prefix(index + 1)) //nolint:gosec
	v.add(index, -1)
	v.slots[index] = 0
	delete(v.pos, uid)
	v.count--

	return seq, true
}

// Count returns the number of messages in the view.
func (v *View) Count() uint32 {
	return v.count
}

// UIDAt returns the UID for a sequence number.
func (v *View) UIDAt(seq uint32) (uint32, bool) {
	index := v.kth(seq)
	if index < 0 {
		return 0, false
	}

	return v.slots[index], true
}

// Seq returns the sequence number for a UID.
func (v *View) Seq(uid uint32) (uint32, bool) {
	index, ok := v.pos[uid]
	if !ok {
		return 0, false
	}

	return uint32(v.prefix(index + 1)), true //nolint:gosec
}

// UIDs returns the current UIDs in sequence number order.
func (v *View) UIDs() []uint32 {
	uids := make([]uint32, 0, v.count)

	for _, uid := range v.slots {
		if uid != 0 {
			uids = append(uids, uid)
		}
	}

	return uids
}

// UIDValidity returns the UIDVALIDITY of the view.
func (v *View) UIDValidity() uint32 {
	return v.uidValidity
}

// HighestUID returns the highest UID ever appended to the view.
func (v *View) HighestUID() uint32 {
	return v.highestUID
}
