	BannedAt time.Time
	BanTime  time.Duration
	Reason   string
}

// TableName returns name of table
func (r NodeBanLogs) TableName() string {
	return "1_node_ban_logs"
}
