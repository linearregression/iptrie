// *** AUTOGENERATED BY "go generate" ***

package iptrie

import (
	"fmt"
	"unsafe"
)

type Trie32 struct {
	node  *Node32
	nodes []Node32
}

type Node32 struct {
	prefixlen byte
	a, b      *Node32
	bits      [32 / 32]uint32
	data      unsafe.Pointer
	dummy     byte
}

// sweep goes thru whole subtree calling f. Could be used for cleanup,
// e.g.  tree.sweep(0, func(_ int, n *node) { n.a, n.b, n.data = nil, nil, nil })
func (node *Node32) Sweep(f func(*Node32)) {
	// reverse order
	if node.a != nil {
		node.a.Sweep(f)
	}
	if node.b != nil {
		node.b.Sweep(f)
	}
	f(node)
}

func (node *Node32) Drill(f func(*Node32)) {
	f(node)
	if node.b != nil {
		node.b.Drill(f)
	}
	if node.a != nil {
		node.a.Drill(f)
	}
}

func (node *Node32) DrillN(f func(*Node32)) {
	stack := []*Node32{node}
	for len(stack) > 0 {
		xn := len(stack) - 1
		f(stack[xn])
		if node.b != nil {
			stack[xn] = node.b
			if node.a != nil {
				stack = append(stack, node.a)
			}
		} else if node.a != nil {
			stack[xn] = node.a
		} else {
			stack = stack[:xn]
		}
	}
}

func (t *Trie32) Root() *Node32 {
	return t.node
}

func (node *Node32) Bits() byte {
	return node.prefixlen
}

func (node *Node32) IP() []byte {
	words := int(node.prefixlen+31) / 32
	s := make([]byte, 4*words)
	for i := 0; i < words; i++ {
		u32, start := node.bits[i], i*4
		s[start], s[start+1], s[start+2], s[start+3] = byte(u32>>24), byte(u32>>16), byte(u32>>8), byte(u32)
	}
	return s
}

// match returns true if key/ln is valid child of node or node itself
func (node *Node32) match(key []byte, ln byte) bool {
	if ln < node.prefixlen {
		return false
	}

	if npl := node.prefixlen; npl != 0 {
		mask := uint32(0xffffffff)
		if npl%32 != 0 {
			mask = ^(mask >> (npl % 32))
		}
		if npl <= 32 {
			return node.bits[0]&mask == mkuint32(key, ln)&mask
		}

		m := (npl - 1) / 32
		if m > 0 {
			for s := m - 1; s > 0; s-- {
				if node.bits[s] != mkuint32(key[s*4:], ln-s*8) {
					return false
				}
			}
			if node.bits[0] != mkuint32(key[0:], ln) {
				return false
			}
		}
		if node.bits[m]&mask != mkuint32(key[m*4:], ln-m*8)&mask {
			return false
		}
	}
	return true
}

func (node *Node32) bitsMatched(key []uint32, ln byte) byte {
	npl := node.prefixlen
	if ln < npl {
		npl = ln // limit matching to min length
	}
	if npl == 0 {
		return 0
	}
	var n, plen byte
	for n = 0; n < npl/32; n++ {
		// how many should be equal?
		if key[n] != node.bits[n] {
			// compare that bit
			break
		}
		plen += 32 // skip checking every bit in this word
	}

	var mask uint32

	for plen < npl {
		mask = (mask >> 1) | 0x80000000 // move 1 and set 32nd bit to 1
		if (node.bits[n] & mask) != (key[n] & mask) {
			break
		}
		plen++
	}

	return plen
}

func (t *Trie32) newnode(bits []byte, prefixlen, dummy byte) *Node32 {
	if len(t.nodes) == 0 {
		t.nodes = make([]Node32, 20) // 20 nodes at once to prepare
	}

	idx := len(t.nodes) - 1
	node := &(t.nodes[idx])
	t.nodes = t.nodes[:idx]

	node.prefixlen, node.dummy = prefixlen, dummy

	end := (prefixlen + 31) / 32
	for pos := byte(0); pos < end; pos++ {
		node.bits[pos] = mkuint32(bits[pos*4:], prefixlen)
		prefixlen -= 32
	}
	return node
}

func (node *Node32) findBestMatch(key []byte, ln byte) (bool, *Node32, *Node32) {
	var (
		exact   bool
		cparent *Node32
		parent  *Node32
	)
	for node != nil && node.match(key, ln) {
		if parent != nil && parent.dummy == 0 {
			cparent = parent
		}
		if DEBUG != nil {
			if node.dummy != 0 {
				fmt.Fprintf(DEBUG, "dummy %s for %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln))
			} else {
				fmt.Fprintf(DEBUG, "found %s for %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln))
			}
		}
		parent = node
		if node.prefixlen == ln {
			exact = true
			break
		}
		if hasBit8(key, parent.prefixlen+1) {
			node = node.a
		} else {
			node = node.b
		}
	}
	return exact, parent, cparent
}

func (node *Node32) delChildNode(key []byte, ln byte) bool {
	var parent *Node32
	for node != nil && node.match(key, ln) {
		parent = node
		if hasBit8(key, parent.prefixlen+1) {
			node = node.a
			if node.prefixlen == ln {
				if node.a == nil && node.b == nil {
					parent.a = nil
				} else if node.b == nil {
					// right branch has right side child, trim
					parent.a = node.a
				} else {
					node.data = nil
					node.dummy = 1
				}
				return true
			}
		} else {
			node = node.b
			if node.prefixlen == ln {
				if node.a == nil && node.b == nil {
					parent.b = nil
				} else if node.a == nil {
					// left branch has left side child, trim
					parent.b = node.b
				} else {
					node.data = nil
					node.dummy = 1
				}
				return true
			}
		}
	}
	return false
}

func (t *Trie32) addToNode(node *Node32, key []byte, ln byte, value unsafe.Pointer, replace bool) (set bool, newnode *Node32) {
	if ln > 32 {
		panic("Unable to add prefix longer than 32")
	}

	set = true
	if t.node == nil {
		// just starting a tree
		if DEBUG != nil {
			fmt.Fprintf(DEBUG, "root=%s (no subtree)\n", keyStr(key, ln))
		}
		t.node = t.newnode(key[:(ln+7)/8], ln, 0)
		t.node.data = value
		newnode = t.node
		return
	}
	var (
		exact bool
		down  *Node32
	)
	if exact, node, _ = node.findBestMatch(key, ln); exact {
		if node.dummy != 0 {
			node.Assign(value)
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "setting empty child's %v/%d value\n", key, ln)
			}
		} else {
			if replace {
				node.data = value
			} else {
				set = false // this is only time we don't set
			}
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "hit previously set %v/%d node\n", key, ln)
			}
		}
		return set, node
	}
	newnode = t.newnode(key, ln, 0)
	newnode.data = value
	if node != nil {
		if hasBit8(key, node.prefixlen+1) {
			if node.a == nil {
				node.a = newnode
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "a-child %s for %s\n", keyStr(key, ln), keyStr(node.IP(), node.prefixlen))
				}
				return set, newnode
			}
			// newnode fits between node and node.a
			down = node.a
		} else {
			if node.b == nil {
				node.b = newnode
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "b-child %s for %s\n", keyStr(key, ln), keyStr(node.IP(), node.prefixlen))
				}
				return set, newnode
			}
			// newnode fits between node and node.b
			down = node.b
		}
	} else {
		// newnode goes in front of root node
		down = t.node
	}

	parent := node
	if parent != nil && parent.prefixlen >= ln {
		panic("parent's prefix could not be larger than key len")
	}

	matched := down.bitsMatched(newnode.bits[:], ln)

	// Well. We fit somewhere between parent and down
	// parent.bits match up to parent.prefixlen          1111111111100000000000
	//                                                   11111111111..11
	// down.bits match up to matched                     11111111111..1111

	if matched == ln {
		// down is child of key
		if hasBit(down.bits[:], ln+1) {
			newnode.a = down
		} else {
			newnode.b = down
		}
		if parent != nil {
			use_a := hasBit(newnode.bits[:], parent.prefixlen+1)
			if use_a != hasBit(down.bits[:], parent.prefixlen+1) {
				panic("something is wrong with branch that we intend to append to")
			}
			if use_a {
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert a-child %s to %s before %s\n", keyStr(key, ln), keyStr(parent.IP(), parent.prefixlen), keyStr(down.IP(), down.prefixlen))
				}
				parent.a = newnode
			} else {
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert b-child %s to %s before %s\n", keyStr(key, ln), keyStr(parent.IP(), parent.prefixlen), keyStr(down.IP(), down.prefixlen))
				}
				parent.b = newnode
			}
		} else {
			if DEBUG != nil {
				m := "b"
				if hasBit(newnode.bits[:], 1) {
					m = "a"
				}
				fmt.Fprintf(DEBUG, "root=%s (uses %s as %s-child)\n", keyStr(key, ln), keyStr(down.IP(), down.prefixlen), m)
			}
			t.node = newnode
		}
	} else {
		// down and newnode should have new dummy parent under parent
		node = t.newnode(key[:(ln+7)/8], matched, 1)
		use_a := hasBit(down.bits[:], matched+1)
		if use_a == hasBit(newnode.bits[:], matched+1) {
			panic("tangled branches while creating new intermediate parent")
		}
		if use_a {
			node.a = down
			node.b = newnode
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "created a-dummy %s with %s and %s\n", keyStr(node.IP(), node.prefixlen), keyStr(down.IP(), down.prefixlen), keyStr(key, ln))
			}
		} else {
			node.b = down
			node.a = newnode
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "created b-dummy %s with %s and %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln), keyStr(down.IP(), down.prefixlen))
			}
		}

		//insert b-child 1.2.3.0/25 to 1.2.3.0/24 before 1.2.3.0/29
		if parent != nil {
			if hasBit(node.bits[:], parent.prefixlen+1) {
				parent.a = node
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert a-child %s to %s before %s\n", keyStr(node.IP(), node.prefixlen), keyStr(parent.IP(), parent.prefixlen), keyStr(node.a.IP(), node.a.prefixlen))
				}
			} else {
				parent.b = node
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert b-child %s to %s before %s\n", keyStr(node.IP(), node.prefixlen), keyStr(parent.IP(), parent.prefixlen), keyStr(node.b.IP(), node.b.prefixlen))
				}
			}
		} else {
			if DEBUG != nil {
				m := "b"
				if use_a {
					m = "a"
				}
				fmt.Fprintf(DEBUG, "root=%s (uses %s as %s-child)\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln), m)
			}
			t.node = node
		}
	}

	return
}

func (rt *Trie32) Get(ip []byte, mask byte) (bool, []byte, byte, unsafe.Pointer) {
	exact, node, ct := rt.node.findBestMatch(ip, mask)

	if node != nil && node.dummy == 0 {
		// dummy=1 means "no match", we will instead look at valid container
		return exact, node.IP(), node.prefixlen, node.data
	}

	if ct != nil {
		// accept container as the answer if it's present
		return false, ct.IP(), ct.prefixlen, ct.data
	}
	return false, nil, 0, nil

}

func (rt *Trie32) Append(ip []byte, mask byte, value unsafe.Pointer) (bool, *Node32) {
	set, olval := rt.addToNode(rt.node, ip, mask, value, false)
	return set, olval
}

func (rt *Trie32) Remove(ip []byte, mask byte) bool {
	return rt.node.delChildNode(ip, mask)
}

func (rt *Trie32) Set(ip []byte, mask byte, value unsafe.Pointer) (bool, *Node32) {
	set, olval := rt.addToNode(rt.node, ip, mask, value, true)
	return set, olval
}

func (rt *Trie32) GetNode(ip []byte, mask byte) (bool, *Node32) {
	exact, node, ct := rt.node.findBestMatch(ip, mask)
	if exact {
		return node.IsDummy(), node // if node is a dummy it needs to look like "just added"
	}
	if node != nil {
		_, node = rt.addToNode(node, ip, mask, nil, false)
	} else {
		if ct != nil {
			_, node = rt.addToNode(ct, ip, mask, nil, false)
		} else {
			_, node = rt.addToNode(rt.node, ip, mask, nil, false)
		}
	}
	return true, node

}

func (n *Node32) Data() unsafe.Pointer {
	return n.data
}

func (n *Node32) IsDummy() bool {
	return n.dummy != 0
}

func (n *Node32) Assign(value unsafe.Pointer) {
	n.data = value
	n.dummy = 0
}

type Trie64 struct {
	node  *Node64
	nodes []Node64
}

type Node64 struct {
	prefixlen byte
	a, b      *Node64
	bits      [64 / 32]uint32
	data      unsafe.Pointer
	dummy     byte
}

// sweep goes thru whole subtree calling f. Could be used for cleanup,
// e.g.  tree.sweep(0, func(_ int, n *node) { n.a, n.b, n.data = nil, nil, nil })
func (node *Node64) Sweep(f func(*Node64)) {
	// reverse order
	if node.a != nil {
		node.a.Sweep(f)
	}
	if node.b != nil {
		node.b.Sweep(f)
	}
	f(node)
}

func (node *Node64) Drill(f func(*Node64)) {
	f(node)
	if node.b != nil {
		node.b.Drill(f)
	}
	if node.a != nil {
		node.a.Drill(f)
	}
}

func (node *Node64) DrillN(f func(*Node64)) {
	stack := []*Node64{node}
	for len(stack) > 0 {
		xn := len(stack) - 1
		f(stack[xn])
		if node.b != nil {
			stack[xn] = node.b
			if node.a != nil {
				stack = append(stack, node.a)
			}
		} else if node.a != nil {
			stack[xn] = node.a
		} else {
			stack = stack[:xn]
		}
	}
}

func (t *Trie64) Root() *Node64 {
	return t.node
}

func (node *Node64) Bits() byte {
	return node.prefixlen
}

func (node *Node64) IP() []byte {
	words := int(node.prefixlen+31) / 32
	s := make([]byte, 4*words)
	for i := 0; i < words; i++ {
		u32, start := node.bits[i], i*4
		s[start], s[start+1], s[start+2], s[start+3] = byte(u32>>24), byte(u32>>16), byte(u32>>8), byte(u32)
	}
	return s
}

// match returns true if key/ln is valid child of node or node itself
func (node *Node64) match(key []byte, ln byte) bool {
	if ln < node.prefixlen {
		return false
	}

	if npl := node.prefixlen; npl != 0 {
		mask := uint32(0xffffffff)
		if npl%32 != 0 {
			mask = ^(mask >> (npl % 32))
		}
		if npl <= 32 {
			return node.bits[0]&mask == mkuint32(key, ln)&mask
		}

		m := (npl - 1) / 32
		if m > 0 {
			for s := m - 1; s > 0; s-- {
				if node.bits[s] != mkuint32(key[s*4:], ln-s*8) {
					return false
				}
			}
			if node.bits[0] != mkuint32(key[0:], ln) {
				return false
			}
		}
		if node.bits[m]&mask != mkuint32(key[m*4:], ln-m*8)&mask {
			return false
		}
	}
	return true
}

func (node *Node64) bitsMatched(key []uint32, ln byte) byte {
	npl := node.prefixlen
	if ln < npl {
		npl = ln // limit matching to min length
	}
	if npl == 0 {
		return 0
	}
	var n, plen byte
	for n = 0; n < npl/32; n++ {
		// how many should be equal?
		if key[n] != node.bits[n] {
			// compare that bit
			break
		}
		plen += 32 // skip checking every bit in this word
	}

	var mask uint32

	for plen < npl {
		mask = (mask >> 1) | 0x80000000 // move 1 and set 32nd bit to 1
		if (node.bits[n] & mask) != (key[n] & mask) {
			break
		}
		plen++
	}

	return plen
}

func (t *Trie64) newnode(bits []byte, prefixlen, dummy byte) *Node64 {
	if len(t.nodes) == 0 {
		t.nodes = make([]Node64, 20) // 20 nodes at once to prepare
	}

	idx := len(t.nodes) - 1
	node := &(t.nodes[idx])
	t.nodes = t.nodes[:idx]

	node.prefixlen, node.dummy = prefixlen, dummy

	end := (prefixlen + 31) / 32
	for pos := byte(0); pos < end; pos++ {
		node.bits[pos] = mkuint32(bits[pos*4:], prefixlen)
		prefixlen -= 32
	}
	return node
}

func (node *Node64) findBestMatch(key []byte, ln byte) (bool, *Node64, *Node64) {
	var (
		exact   bool
		cparent *Node64
		parent  *Node64
	)
	for node != nil && node.match(key, ln) {
		if parent != nil && parent.dummy == 0 {
			cparent = parent
		}
		if DEBUG != nil {
			if node.dummy != 0 {
				fmt.Fprintf(DEBUG, "dummy %s for %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln))
			} else {
				fmt.Fprintf(DEBUG, "found %s for %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln))
			}
		}
		parent = node
		if node.prefixlen == ln {
			exact = true
			break
		}
		if hasBit8(key, parent.prefixlen+1) {
			node = node.a
		} else {
			node = node.b
		}
	}
	return exact, parent, cparent
}

func (node *Node64) delChildNode(key []byte, ln byte) bool {
	var parent *Node64
	for node != nil && node.match(key, ln) {
		parent = node
		if hasBit8(key, parent.prefixlen+1) {
			node = node.a
			if node.prefixlen == ln {
				if node.a == nil && node.b == nil {
					parent.a = nil
				} else if node.b == nil {
					// right branch has right side child, trim
					parent.a = node.a
				} else {
					node.data = nil
					node.dummy = 1
				}
				return true
			}
		} else {
			node = node.b
			if node.prefixlen == ln {
				if node.a == nil && node.b == nil {
					parent.b = nil
				} else if node.a == nil {
					// left branch has left side child, trim
					parent.b = node.b
				} else {
					node.data = nil
					node.dummy = 1
				}
				return true
			}
		}
	}
	return false
}

func (t *Trie64) addToNode(node *Node64, key []byte, ln byte, value unsafe.Pointer, replace bool) (set bool, newnode *Node64) {
	if ln > 64 {
		panic("Unable to add prefix longer than 64")
	}

	set = true
	if t.node == nil {
		// just starting a tree
		if DEBUG != nil {
			fmt.Fprintf(DEBUG, "root=%s (no subtree)\n", keyStr(key, ln))
		}
		t.node = t.newnode(key[:(ln+7)/8], ln, 0)
		t.node.data = value
		newnode = t.node
		return
	}
	var (
		exact bool
		down  *Node64
	)
	if exact, node, _ = node.findBestMatch(key, ln); exact {
		if node.dummy != 0 {
			node.Assign(value)
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "setting empty child's %v/%d value\n", key, ln)
			}
		} else {
			if replace {
				node.data = value
			} else {
				set = false // this is only time we don't set
			}
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "hit previously set %v/%d node\n", key, ln)
			}
		}
		return set, node
	}
	newnode = t.newnode(key, ln, 0)
	newnode.data = value
	if node != nil {
		if hasBit8(key, node.prefixlen+1) {
			if node.a == nil {
				node.a = newnode
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "a-child %s for %s\n", keyStr(key, ln), keyStr(node.IP(), node.prefixlen))
				}
				return set, newnode
			}
			// newnode fits between node and node.a
			down = node.a
		} else {
			if node.b == nil {
				node.b = newnode
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "b-child %s for %s\n", keyStr(key, ln), keyStr(node.IP(), node.prefixlen))
				}
				return set, newnode
			}
			// newnode fits between node and node.b
			down = node.b
		}
	} else {
		// newnode goes in front of root node
		down = t.node
	}

	parent := node
	if parent != nil && parent.prefixlen >= ln {
		panic("parent's prefix could not be larger than key len")
	}

	matched := down.bitsMatched(newnode.bits[:], ln)

	// Well. We fit somewhere between parent and down
	// parent.bits match up to parent.prefixlen          1111111111100000000000
	//                                                   11111111111..11
	// down.bits match up to matched                     11111111111..1111

	if matched == ln {
		// down is child of key
		if hasBit(down.bits[:], ln+1) {
			newnode.a = down
		} else {
			newnode.b = down
		}
		if parent != nil {
			use_a := hasBit(newnode.bits[:], parent.prefixlen+1)
			if use_a != hasBit(down.bits[:], parent.prefixlen+1) {
				panic("something is wrong with branch that we intend to append to")
			}
			if use_a {
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert a-child %s to %s before %s\n", keyStr(key, ln), keyStr(parent.IP(), parent.prefixlen), keyStr(down.IP(), down.prefixlen))
				}
				parent.a = newnode
			} else {
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert b-child %s to %s before %s\n", keyStr(key, ln), keyStr(parent.IP(), parent.prefixlen), keyStr(down.IP(), down.prefixlen))
				}
				parent.b = newnode
			}
		} else {
			if DEBUG != nil {
				m := "b"
				if hasBit(newnode.bits[:], 1) {
					m = "a"
				}
				fmt.Fprintf(DEBUG, "root=%s (uses %s as %s-child)\n", keyStr(key, ln), keyStr(down.IP(), down.prefixlen), m)
			}
			t.node = newnode
		}
	} else {
		// down and newnode should have new dummy parent under parent
		node = t.newnode(key[:(ln+7)/8], matched, 1)
		use_a := hasBit(down.bits[:], matched+1)
		if use_a == hasBit(newnode.bits[:], matched+1) {
			panic("tangled branches while creating new intermediate parent")
		}
		if use_a {
			node.a = down
			node.b = newnode
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "created a-dummy %s with %s and %s\n", keyStr(node.IP(), node.prefixlen), keyStr(down.IP(), down.prefixlen), keyStr(key, ln))
			}
		} else {
			node.b = down
			node.a = newnode
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "created b-dummy %s with %s and %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln), keyStr(down.IP(), down.prefixlen))
			}
		}

		//insert b-child 1.2.3.0/25 to 1.2.3.0/24 before 1.2.3.0/29
		if parent != nil {
			if hasBit(node.bits[:], parent.prefixlen+1) {
				parent.a = node
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert a-child %s to %s before %s\n", keyStr(node.IP(), node.prefixlen), keyStr(parent.IP(), parent.prefixlen), keyStr(node.a.IP(), node.a.prefixlen))
				}
			} else {
				parent.b = node
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert b-child %s to %s before %s\n", keyStr(node.IP(), node.prefixlen), keyStr(parent.IP(), parent.prefixlen), keyStr(node.b.IP(), node.b.prefixlen))
				}
			}
		} else {
			if DEBUG != nil {
				m := "b"
				if use_a {
					m = "a"
				}
				fmt.Fprintf(DEBUG, "root=%s (uses %s as %s-child)\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln), m)
			}
			t.node = node
		}
	}

	return
}

func (rt *Trie64) Get(ip []byte, mask byte) (bool, []byte, byte, unsafe.Pointer) {
	exact, node, ct := rt.node.findBestMatch(ip, mask)

	if node != nil && node.dummy == 0 {
		// dummy=1 means "no match", we will instead look at valid container
		return exact, node.IP(), node.prefixlen, node.data
	}

	if ct != nil {
		// accept container as the answer if it's present
		return false, ct.IP(), ct.prefixlen, ct.data
	}
	return false, nil, 0, nil

}

func (rt *Trie64) Append(ip []byte, mask byte, value unsafe.Pointer) (bool, *Node64) {
	set, olval := rt.addToNode(rt.node, ip, mask, value, false)
	return set, olval
}

func (rt *Trie64) Remove(ip []byte, mask byte) bool {
	return rt.node.delChildNode(ip, mask)
}

func (rt *Trie64) Set(ip []byte, mask byte, value unsafe.Pointer) (bool, *Node64) {
	set, olval := rt.addToNode(rt.node, ip, mask, value, true)
	return set, olval
}

func (rt *Trie64) GetNode(ip []byte, mask byte) (bool, *Node64) {
	exact, node, ct := rt.node.findBestMatch(ip, mask)
	if exact {
		return node.IsDummy(), node // if node is a dummy it needs to look like "just added"
	}
	if node != nil {
		_, node = rt.addToNode(node, ip, mask, nil, false)
	} else {
		if ct != nil {
			_, node = rt.addToNode(ct, ip, mask, nil, false)
		} else {
			_, node = rt.addToNode(rt.node, ip, mask, nil, false)
		}
	}
	return true, node

}

func (n *Node64) Data() unsafe.Pointer {
	return n.data
}

func (n *Node64) IsDummy() bool {
	return n.dummy != 0
}

func (n *Node64) Assign(value unsafe.Pointer) {
	n.data = value
	n.dummy = 0
}

type Trie128 struct {
	node  *Node128
	nodes []Node128
}

type Node128 struct {
	prefixlen byte
	a, b      *Node128
	bits      [128 / 32]uint32
	data      unsafe.Pointer
	dummy     byte
}

// sweep goes thru whole subtree calling f. Could be used for cleanup,
// e.g.  tree.sweep(0, func(_ int, n *node) { n.a, n.b, n.data = nil, nil, nil })
func (node *Node128) Sweep(f func(*Node128)) {
	// reverse order
	if node.a != nil {
		node.a.Sweep(f)
	}
	if node.b != nil {
		node.b.Sweep(f)
	}
	f(node)
}

func (node *Node128) Drill(f func(*Node128)) {
	f(node)
	if node.b != nil {
		node.b.Drill(f)
	}
	if node.a != nil {
		node.a.Drill(f)
	}
}

func (node *Node128) DrillN(f func(*Node128)) {
	stack := []*Node128{node}
	for len(stack) > 0 {
		xn := len(stack) - 1
		f(stack[xn])
		if node.b != nil {
			stack[xn] = node.b
			if node.a != nil {
				stack = append(stack, node.a)
			}
		} else if node.a != nil {
			stack[xn] = node.a
		} else {
			stack = stack[:xn]
		}
	}
}

func (t *Trie128) Root() *Node128 {
	return t.node
}

func (node *Node128) Bits() byte {
	return node.prefixlen
}

func (node *Node128) IP() []byte {
	words := int(node.prefixlen+31) / 32
	s := make([]byte, 4*words)
	for i := 0; i < words; i++ {
		u32, start := node.bits[i], i*4
		s[start], s[start+1], s[start+2], s[start+3] = byte(u32>>24), byte(u32>>16), byte(u32>>8), byte(u32)
	}
	return s
}

// match returns true if key/ln is valid child of node or node itself
func (node *Node128) match(key []byte, ln byte) bool {
	if ln < node.prefixlen {
		return false
	}

	if npl := node.prefixlen; npl != 0 {
		mask := uint32(0xffffffff)
		if npl%32 != 0 {
			mask = ^(mask >> (npl % 32))
		}
		if npl <= 32 {
			return node.bits[0]&mask == mkuint32(key, ln)&mask
		}

		m := (npl - 1) / 32
		if m > 0 {
			for s := m - 1; s > 0; s-- {
				if node.bits[s] != mkuint32(key[s*4:], ln-s*8) {
					return false
				}
			}
			if node.bits[0] != mkuint32(key[0:], ln) {
				return false
			}
		}
		if node.bits[m]&mask != mkuint32(key[m*4:], ln-m*8)&mask {
			return false
		}
	}
	return true
}

func (node *Node128) bitsMatched(key []uint32, ln byte) byte {
	npl := node.prefixlen
	if ln < npl {
		npl = ln // limit matching to min length
	}
	if npl == 0 {
		return 0
	}
	var n, plen byte
	for n = 0; n < npl/32; n++ {
		// how many should be equal?
		if key[n] != node.bits[n] {
			// compare that bit
			break
		}
		plen += 32 // skip checking every bit in this word
	}

	var mask uint32

	for plen < npl {
		mask = (mask >> 1) | 0x80000000 // move 1 and set 32nd bit to 1
		if (node.bits[n] & mask) != (key[n] & mask) {
			break
		}
		plen++
	}

	return plen
}

func (t *Trie128) newnode(bits []byte, prefixlen, dummy byte) *Node128 {
	if len(t.nodes) == 0 {
		t.nodes = make([]Node128, 20) // 20 nodes at once to prepare
	}

	idx := len(t.nodes) - 1
	node := &(t.nodes[idx])
	t.nodes = t.nodes[:idx]

	node.prefixlen, node.dummy = prefixlen, dummy

	end := (prefixlen + 31) / 32
	for pos := byte(0); pos < end; pos++ {
		node.bits[pos] = mkuint32(bits[pos*4:], prefixlen)
		prefixlen -= 32
	}
	return node
}

func (node *Node128) findBestMatch(key []byte, ln byte) (bool, *Node128, *Node128) {
	var (
		exact   bool
		cparent *Node128
		parent  *Node128
	)
	for node != nil && node.match(key, ln) {
		if parent != nil && parent.dummy == 0 {
			cparent = parent
		}
		if DEBUG != nil {
			if node.dummy != 0 {
				fmt.Fprintf(DEBUG, "dummy %s for %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln))
			} else {
				fmt.Fprintf(DEBUG, "found %s for %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln))
			}
		}
		parent = node
		if node.prefixlen == ln {
			exact = true
			break
		}
		if hasBit8(key, parent.prefixlen+1) {
			node = node.a
		} else {
			node = node.b
		}
	}
	return exact, parent, cparent
}

func (node *Node128) delChildNode(key []byte, ln byte) bool {
	var parent *Node128
	for node != nil && node.match(key, ln) {
		parent = node
		if hasBit8(key, parent.prefixlen+1) {
			node = node.a
			if node.prefixlen == ln {
				if node.a == nil && node.b == nil {
					parent.a = nil
				} else if node.b == nil {
					// right branch has right side child, trim
					parent.a = node.a
				} else {
					node.data = nil
					node.dummy = 1
				}
				return true
			}
		} else {
			node = node.b
			if node.prefixlen == ln {
				if node.a == nil && node.b == nil {
					parent.b = nil
				} else if node.a == nil {
					// left branch has left side child, trim
					parent.b = node.b
				} else {
					node.data = nil
					node.dummy = 1
				}
				return true
			}
		}
	}
	return false
}

func (t *Trie128) addToNode(node *Node128, key []byte, ln byte, value unsafe.Pointer, replace bool) (set bool, newnode *Node128) {
	if ln > 128 {
		panic("Unable to add prefix longer than 128")
	}

	set = true
	if t.node == nil {
		// just starting a tree
		if DEBUG != nil {
			fmt.Fprintf(DEBUG, "root=%s (no subtree)\n", keyStr(key, ln))
		}
		t.node = t.newnode(key[:(ln+7)/8], ln, 0)
		t.node.data = value
		newnode = t.node
		return
	}
	var (
		exact bool
		down  *Node128
	)
	if exact, node, _ = node.findBestMatch(key, ln); exact {
		if node.dummy != 0 {
			node.Assign(value)
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "setting empty child's %v/%d value\n", key, ln)
			}
		} else {
			if replace {
				node.data = value
			} else {
				set = false // this is only time we don't set
			}
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "hit previously set %v/%d node\n", key, ln)
			}
		}
		return set, node
	}
	newnode = t.newnode(key, ln, 0)
	newnode.data = value
	if node != nil {
		if hasBit8(key, node.prefixlen+1) {
			if node.a == nil {
				node.a = newnode
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "a-child %s for %s\n", keyStr(key, ln), keyStr(node.IP(), node.prefixlen))
				}
				return set, newnode
			}
			// newnode fits between node and node.a
			down = node.a
		} else {
			if node.b == nil {
				node.b = newnode
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "b-child %s for %s\n", keyStr(key, ln), keyStr(node.IP(), node.prefixlen))
				}
				return set, newnode
			}
			// newnode fits between node and node.b
			down = node.b
		}
	} else {
		// newnode goes in front of root node
		down = t.node
	}

	parent := node
	if parent != nil && parent.prefixlen >= ln {
		panic("parent's prefix could not be larger than key len")
	}

	matched := down.bitsMatched(newnode.bits[:], ln)

	// Well. We fit somewhere between parent and down
	// parent.bits match up to parent.prefixlen          1111111111100000000000
	//                                                   11111111111..11
	// down.bits match up to matched                     11111111111..1111

	if matched == ln {
		// down is child of key
		if hasBit(down.bits[:], ln+1) {
			newnode.a = down
		} else {
			newnode.b = down
		}
		if parent != nil {
			use_a := hasBit(newnode.bits[:], parent.prefixlen+1)
			if use_a != hasBit(down.bits[:], parent.prefixlen+1) {
				panic("something is wrong with branch that we intend to append to")
			}
			if use_a {
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert a-child %s to %s before %s\n", keyStr(key, ln), keyStr(parent.IP(), parent.prefixlen), keyStr(down.IP(), down.prefixlen))
				}
				parent.a = newnode
			} else {
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert b-child %s to %s before %s\n", keyStr(key, ln), keyStr(parent.IP(), parent.prefixlen), keyStr(down.IP(), down.prefixlen))
				}
				parent.b = newnode
			}
		} else {
			if DEBUG != nil {
				m := "b"
				if hasBit(newnode.bits[:], 1) {
					m = "a"
				}
				fmt.Fprintf(DEBUG, "root=%s (uses %s as %s-child)\n", keyStr(key, ln), keyStr(down.IP(), down.prefixlen), m)
			}
			t.node = newnode
		}
	} else {
		// down and newnode should have new dummy parent under parent
		node = t.newnode(key[:(ln+7)/8], matched, 1)
		use_a := hasBit(down.bits[:], matched+1)
		if use_a == hasBit(newnode.bits[:], matched+1) {
			panic("tangled branches while creating new intermediate parent")
		}
		if use_a {
			node.a = down
			node.b = newnode
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "created a-dummy %s with %s and %s\n", keyStr(node.IP(), node.prefixlen), keyStr(down.IP(), down.prefixlen), keyStr(key, ln))
			}
		} else {
			node.b = down
			node.a = newnode
			if DEBUG != nil {
				fmt.Fprintf(DEBUG, "created b-dummy %s with %s and %s\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln), keyStr(down.IP(), down.prefixlen))
			}
		}

		//insert b-child 1.2.3.0/25 to 1.2.3.0/24 before 1.2.3.0/29
		if parent != nil {
			if hasBit(node.bits[:], parent.prefixlen+1) {
				parent.a = node
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert a-child %s to %s before %s\n", keyStr(node.IP(), node.prefixlen), keyStr(parent.IP(), parent.prefixlen), keyStr(node.a.IP(), node.a.prefixlen))
				}
			} else {
				parent.b = node
				if DEBUG != nil {
					fmt.Fprintf(DEBUG, "insert b-child %s to %s before %s\n", keyStr(node.IP(), node.prefixlen), keyStr(parent.IP(), parent.prefixlen), keyStr(node.b.IP(), node.b.prefixlen))
				}
			}
		} else {
			if DEBUG != nil {
				m := "b"
				if use_a {
					m = "a"
				}
				fmt.Fprintf(DEBUG, "root=%s (uses %s as %s-child)\n", keyStr(node.IP(), node.prefixlen), keyStr(key, ln), m)
			}
			t.node = node
		}
	}

	return
}

func (rt *Trie128) Get(ip []byte, mask byte) (bool, []byte, byte, unsafe.Pointer) {
	exact, node, ct := rt.node.findBestMatch(ip, mask)

	if node != nil && node.dummy == 0 {
		// dummy=1 means "no match", we will instead look at valid container
		return exact, node.IP(), node.prefixlen, node.data
	}

	if ct != nil {
		// accept container as the answer if it's present
		return false, ct.IP(), ct.prefixlen, ct.data
	}
	return false, nil, 0, nil

}

func (rt *Trie128) Append(ip []byte, mask byte, value unsafe.Pointer) (bool, *Node128) {
	set, olval := rt.addToNode(rt.node, ip, mask, value, false)
	return set, olval
}

func (rt *Trie128) Remove(ip []byte, mask byte) bool {
	return rt.node.delChildNode(ip, mask)
}

func (rt *Trie128) Set(ip []byte, mask byte, value unsafe.Pointer) (bool, *Node128) {
	set, olval := rt.addToNode(rt.node, ip, mask, value, true)
	return set, olval
}

func (rt *Trie128) GetNode(ip []byte, mask byte) (bool, *Node128) {
	exact, node, ct := rt.node.findBestMatch(ip, mask)
	if exact {
		return node.IsDummy(), node // if node is a dummy it needs to look like "just added"
	}
	if node != nil {
		_, node = rt.addToNode(node, ip, mask, nil, false)
	} else {
		if ct != nil {
			_, node = rt.addToNode(ct, ip, mask, nil, false)
		} else {
			_, node = rt.addToNode(rt.node, ip, mask, nil, false)
		}
	}
	return true, node

}

func (n *Node128) Data() unsafe.Pointer {
	return n.data
}

func (n *Node128) IsDummy() bool {
	return n.dummy != 0
}

func (n *Node128) Assign(value unsafe.Pointer) {
	n.data = value
	n.dummy = 0
}
