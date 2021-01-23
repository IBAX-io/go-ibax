package model

type MintPoolTransferHistory struct {
	ID                       int64  `gorm:"not null"`
	Poolid                   int64  `gorm:"not null"`
	Keyid                    int64  `gorm:"not null"`
	Newkeyid                 int64  `gorm:"not null"`
	Status                   int64  `gorm:"not null"`
	MintTransfersHistoryHash []byte `gorm:"not null"`
	DateUpdated              int64  `gorm:"not null"`
	DateCreated              int64  `gorm:"not null"`
}

}
