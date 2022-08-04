package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/shopspring/decimal"
)

type CandidateNode struct {
	ID              int64           `gorm:"column:id" json:"id"`
	ApiAddress      string          `gorm:"column:api_address" json:"apiAddress"`
	TcpAddress      string          `gorm:"column:tcp_address" json:"tcpAddress"`
	NodePubKey      string          `gorm:"column:node_pub_key" json:"nodePubKey"`
	DateCreated     int64           `gorm:"column:date_created" json:"dateCreated"`
	Deleted         uint8           `gorm:"column:deleted" json:"deleted"`
	DateDeleted     int64           `gorm:"column:date_deleted" json:"dateDeleted"`
	Website         string          `gorm:"column:website" json:"website"`
	ReplyCount      int64           `gorm:"column:reply_count" json:"replyCount"`
	DateReply       int64           `gorm:"column:date_reply" json:"dateReply"`
	EarnestTotal    decimal.Decimal `gorm:"column:earnest_total" json:"earnestTotal"`
	NodeName        string          `gorm:"column:node_name" json:"nodeName"`
	ReferendumTotal decimal.Decimal `gorm:"column:referendum_total" json:"referendumTotal"`
	CandidateNodes  []byte          `json:"candidateNodes"`
}

type CandidateNodes []CandidateNode

func (nodes CandidateNodes) Len() int {
	return len(nodes)
}
func (nodes CandidateNodes) Swap(i, j int) {
	nodes[i], nodes[j] = nodes[j], nodes[i]
}

func (nodes CandidateNodes) Less(i, j int) bool {
	return nodes[i].DateReply > nodes[j].DateReply
}

// TableName returns name of table
func (ib *CandidateNode) TableName() string {
	return "1_candidate_node_requests"
}

// GetCandidateNode returns last good block
func GetCandidateNode(numberOfNodes int) (CandidateNodes, error) {
	var candidateNodes CandidateNodes
	pledgeAmount, err := GetPledgeAmount()
	if err != nil {
		return nil, err
	}
	err = GetDB(nil).Where("deleted = ? and earnest_total >= ?", 0, pledgeAmount).Order("referendum_total desc,date_updated_referendum asc,reply_count desc,date_reply desc").Limit(numberOfNodes).Find(&candidateNodes).Error
	if err != nil {
		return nil, err
	}
	return candidateNodes, nil
}

func (c *CandidateNode) UpdateCandidateNodeInfo() error {
	pledgeAmount, err := GetPledgeAmount()
	if err != nil {
		return err
	}
	err = GetDB(nil).Model(&c).Where("tcp_address = ? and deleted = ? and earnest_total >= ?", c.TcpAddress, 0, pledgeAmount).Updates(CandidateNode{ReplyCount: c.ReplyCount, DateReply: c.DateReply, CandidateNodes: c.CandidateNodes}).Error
	if err != nil {
		return err
	}
	return nil
}

func (c *CandidateNode) GetCandidateNodeByAddress(tcpAddress string) error {
	pledgeAmount, err := GetPledgeAmount()
	if err != nil {
		return err
	}
	err = GetDB(nil).Where("tcp_address = ? and deleted = ? and earnest_total >= ?", tcpAddress, 0, pledgeAmount).Find(&c).Error
	if err != nil {
		return err
	}
	return nil
}

func (c *CandidateNode) GetCandidateNodeByPublicKey(nodePublicKey string) error {
	pledgeAmount, err := GetPledgeAmount()
	if err != nil {
		return err
	}
	err = GetDB(nil).Where("node_pub_key = ? and deleted = ? and earnest_total >= ?", nodePublicKey, 0, pledgeAmount).Find(&c).Error
	if err != nil {
		return err
	}
	return nil
}

func (c *CandidateNode) GetCandidateNodeById(id int64) error {
	pledgeAmount, err := GetPledgeAmount()
	if err != nil {
		return err
	}

	err = GetDB(nil).Where("id = ? and deleted = ? and earnest_total >= ?", id, 0, pledgeAmount).First(&c).Error
	if err != nil {
		return err
	}
	return nil
}

func GetPledgeAmount() (int64, error) {
	var (
		pledgeAmount string
		err          error
		amount       decimal.Decimal
	)
	row := DBConn.Raw(`select
								ap.value 
							from
								"1_app_params" ap,
								"1_applications" a
							where
								ap.app_id = a.id
								and a."name" = 'CandidateNode'
								and ap."name" = 'limit_candidate_pack'
								and a.deleted = 0
								and a.ecosystem = 1`).Row()
	err = row.Scan(&pledgeAmount)
	if err != nil {
		return 0, err
	}
	amount, err = decimal.NewFromString(pledgeAmount)
	if err != nil {
		return 0, err
	}
	return amount.Mul(decimal.New(1, consts.MoneyDigits)).IntPart(), nil
}
