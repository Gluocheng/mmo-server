package runtime

import "github.com/example/mmo-server/gameconfig/pkg/schema"

// StatDef 是一条属性登记。Settle 为 false 时不能挂到有效 Buff 上。
type StatDef struct {
	ID           int32
	Name         string
	AllowFlat    bool
	AllowPercent bool
	Settle       bool
}

// StatByName 按名字读取属性。没有登记时 ok 为 false。
func StatByName(name string) (StatDef, bool) {
	s := getSnapshot()
	if s == nil || s.tables == nil || name == "" {
		return StatDef{}, false
	}
	def, ok := s.tables.stats[name]
	return def, ok
}

// BuildStats 用内存行替换属性名单，保留其它表。仅测试或启动前灌入。
func BuildStats(rows []StatDef) {
	s := getSnapshot()
	var base *tables
	ver := int64(0)
	tc := int32(0)
	if s != nil {
		ver = s.version
		tc = s.tableCount
		base = s.tables.clone()
	} else {
		base = &tables{}
	}
	m := make(map[string]StatDef, len(rows))
	for _, row := range rows {
		if row.Name == "" {
			continue
		}
		m[row.Name] = row
	}
	base.stats = m
	swapSnapshot(&snapshot{version: ver, tableCount: tc, tables: base})
}

func statsFromSchema(rows []schema.CfgStat) map[string]StatDef {
	m := make(map[string]StatDef, len(rows))
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		m[r.Name] = StatDef{
			ID: r.ID, Name: r.Name, AllowFlat: r.AllowFlat, AllowPercent: r.AllowPercent, Settle: r.Settle,
		}
	}
	return m
}
