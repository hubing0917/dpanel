// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.

package entity

const TableNameRunEnv = "ims_run_env"

// RunEnv mapped from table <ims_run_env>
type RunEnv struct {
	ID         int32  `gorm:"column:id;type:INTEGER" json:"id"`
	Name       string `gorm:"column:name" json:"name"`
	Lang       string `gorm:"column:lang" json:"lang"`
	ImageBase  string `gorm:"column:image_base" json:"imageBase"`
	Extra1     string `gorm:"column:extra_1" json:"extra1"`
	Extra2     string `gorm:"column:extra_2" json:"extra2"`
	Extra3     string `gorm:"column:extra_3" json:"extra3"`
	ColumnName int32  `gorm:"column:column_name" json:"columnName"`
}

// TableName RunEnv's table name
func (*RunEnv) TableName() string {
	return TableNameRunEnv
}
