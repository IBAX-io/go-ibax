/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
	ValidateCount int64  `gorm:"not null" json:"nodesCount,omitempty"`
	AppID         int64  `gorm:"column:app_id;not null" json:"app_id,omitempty"`
	Conditions    string `gorm:"not null" json:"conditions,omitempty"`
}

// SetTablePrefix is setting table prefix
func (p *Page) SetTablePrefix(prefix string) {
	p.ecosystem = converter.StrToInt64(prefix)
}

// TableName returns name of table
func (p *Page) TableName() string {
	if p.ecosystem == 0 {
		p.ecosystem = 1
	}
	return `1_pages`
}

// Get is retrieving model from database
func (p *Page) Get(name string) (bool, error) {
	return isFound(DBConn.Where("ecosystem=? and name = ?", p.ecosystem, name).First(p))
}

// Count returns count of records in table
func (p *Page) Count() (count int64, err error) {
	err = DBConn.Table(p.TableName()).Count(&count).Error
	return
}

// GetByApp returns all pages belonging to selected app
func (p *Page) GetByApp(appID int64, ecosystemID int64) ([]Page, error) {
	var result []Page
	err := DBConn.Select("id, name").Where("app_id = ? and ecosystem = ?", appID, ecosystemID).Find(&result).Error
	return result, err
}
