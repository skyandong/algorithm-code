package stack

import "container/list"

type Stack struct {
	l *list.List
}

func NewStack() *Stack {
	s := new(Stack)
	s.l = list.New()
	return s
}

func (s *Stack) Push(val interface{}) {
	s.l.PushFront(val)
}

func (s *Stack) Pop() {
	if s.l.Len() > 0 {
		s.l.Remove(s.l.Front())
	}
}

func (s *Stack) Top() (val interface{}) {
	if s.l.Len() > 0 {
		val = s.l.Front().Value
	}
	return
}

func (s *Stack) Empty() bool {
	if s.l.Len() <= 0 {
		return true
	} else {
		return false
	}
}
