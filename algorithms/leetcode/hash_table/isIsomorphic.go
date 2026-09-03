package hashtable

// 同构字符串
// https://leetcode.cn/problems/isomorphic-strings
func isIsomorphic(s string, t string) bool {
	sTot := map[byte]byte{}
	tTos := map[byte]byte{}

	for i := range s {
		sb, tb := s[i], t[i]
		if (sTot[sb] > 0 && sTot[sb] != tb) || (tTos[tb] > 0 && tTos[tb] != sb) {
			return false
		}
		sTot[s[i]] = tb
		tTos[t[i]] = sb
	}
	return true
}
