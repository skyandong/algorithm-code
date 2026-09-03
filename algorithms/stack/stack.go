package stack

// Stack 是基于切片的栈，Push/Pop/Top 均为均摊 O(1)。
type Stack struct {
	s []any
}

func NewStack() *Stack {
	return &Stack{}
}

func (s *Stack) Push(val any) {
	s.s = append(s.s, val)
}

func (s *Stack) Pop() (val any) {
	if n := len(s.s); n > 0 {
		val = s.s[n-1]
		s.s = s.s[:n-1]
	}
	return
}

func (s *Stack) Top() (val any) {
	if n := len(s.s); n > 0 {
		val = s.s[n-1]
	}
	return
}

func (s *Stack) Empty() bool {
	return len(s.s) == 0
}

func (s *Stack) Len() int {
	return len(s.s)
}
