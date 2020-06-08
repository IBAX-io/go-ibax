package model
	var mp []MineInvitepowadd
	err := GetDB(dbt).Table(m.TableName()).
		Where("stime <= ? and etime >=? ", time, time).
		Order("devid asc").
		Find(&mp).Error
	return mp, err
}
