package protocol

// StringKeyValue 用于跨节点同步网关 session 字段
type StringKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type None struct{}
