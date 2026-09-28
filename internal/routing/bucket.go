package routing

type Bucket struct {
	contacts []*Contact
	k        int
}

func newBucket(k int) *Bucket { return &Bucket{k: k} }

func (b *Bucket) Find(id NodeID) int {
	for i, c := range b.contacts {
		if c.NodeID == id {
			return i
		}
	}
	return -1
}

func (b *Bucket) MoveToEnd(i int) {
	c := b.contacts[i]
	b.contacts = append(b.contacts[:i], b.contacts[i+1:]...)
	b.contacts = append(b.contacts, c)
}

func (b *Bucket) IsFull() bool      { return len(b.contacts) >= b.k }
func (b *Bucket) LRU() *Contact     { return b.contacts[0] }
func (b *Bucket) Append(c *Contact) { b.contacts = append(b.contacts, c) }
func (b *Bucket) Len() int          { return len(b.contacts) }
