package linkedlist

type Node struct {
	Val    int
	Next   *Node
	Random *Node
}

// 随机链表的复制
// https://leetcode.cn/problems/copy-list-with-random-pointer/description/
func copyRandomList(head *Node) *Node {
	m := make(map[*Node]*Node)
	newHead := new(Node)

	for cur := newHead; head != nil; {
		v, ok := m[head]
		if !ok {
			n := &Node{
				Val: head.Val,
			}
			m[head] = n
			v = n
		}
		cur.Next = v

		if head.Random != nil {
			v, ok := m[head.Random]
			if !ok {
				n := &Node{
					Val: head.Random.Val,
				}
				m[head.Random] = n
				v = n
			}
			cur.Next.Random = v
		}

		head = head.Next
		cur = cur.Next
	}

	return newHead.Next
}
