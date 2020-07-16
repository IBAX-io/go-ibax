/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package api

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/IBAX-io/go-ibax/packages/converter"

	"github.com/360EntSecGroup-Skylar/excelize"

	"github.com/IBAX-io/go-ibax/packages/smart"

	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/miner"

	"github.com/stretchr/testify/assert"
)

func TestMintImports(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	var rnd string
	var form url.Values

	rnd = `@1NewColumn`
	form = url.Values{
		`Name`:       {`mine_lock`},
		`ReadPerm`:   {`true`},
		`TableName`:  {`keys`},
		`Type`:       {`number`},
		`UpdatePerm`: {`ContractAccess("@1NewMineStake","@1WithdrawMineStake","@1ReviewMineStake")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1NewColumn`
	form = url.Values{
		`Name`:       {`pool_lock`},
		`ReadPerm`:   {`true`},
		`TableName`:  {`keys`},
		`Type`:       {`number`},
		`UpdatePerm`: {`ContractAccess("@1NewPoolRequest","@1RegainPoolStake","@1SwitchPoolDecision")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1NewColumn`
	form = url.Values{
		`Name`:       {`mintsurplus`},
		`ReadPerm`:   {`true`},
		`TableName`:  {`keys`},
		`Type`:       {`number`},
		`UpdatePerm`: {`ContractAccess("@1Mint","@1SettlementToMinter")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditTable`
	form = url.Values{
		`InsertPerm`:    {`true`},
		`Name`:          {`keys`},
		`NewColumnPerm`: {`ContractConditions("@1AdminCondition")`},
		`ReadPerm`:      {`true`},
		`UpdatePerm`:    {`ContractAccess("@1TokensTransfer","@1TokensLockoutMember","@1MultiwalletCreate","@1NewToken","@1TeBurn","@1TokensDecDeposit","@1TokensIncDeposit","@1ProfileEdit","@1NewUser","@1GetAssignAvailableAmount","@1Mint","@1NewMineStake","@1WithdrawMineStake","@1ReviewMineStake","@1NewPoolRequest","@1RegainPoolStake","@1SwitchPoolDecision","@1SettlementToMinter")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditColumn`
	form = url.Values{
		`Name`:       {`amount`},
		`ReadPerm`:   {`true`},
		`TableName`:  {`keys`},
		`UpdatePerm`: {`ContractAccess("@1TokensTransfer","@1NewToken","@1TeBurn","@1ProfileEdit","@1GetAssignAvailableAmount","@1Mint","@1NewMineStake","@1WithdrawMineStake","@1ReviewMineStake","@1NewPoolRequest","@1RegainPoolStake","@1SwitchPoolDecision","@1SettlementToMinter")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditTable`
	form = url.Values{
		`InsertPerm`:    {`ContractAccess("@1TokensTransfer","@1NewToken","@1TeBurn","@1ProfileEdit","@1GetAssignAvailableAmount","@1Mint","@1NewMineStake","@1WithdrawMineStake","@1ReviewMineStake","@1NewPoolRequest","@1RegainPoolStake","@1SwitchPoolDecision","@1SettlementToMinter")`},
		`Name`:          {`history`},
		`NewColumnPerm`: {`ContractConditions("@1AdminCondition")`},
		`ReadPerm`:      {`true`},
		`UpdatePerm`:    {`ContractConditions("@1AdminCondition")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditTable`
	form = url.Values{
		`InsertPerm`:    {`ContractConditions("DeveloperCondition")`},
		`Name`:          {`parameters`},
		`NewColumnPerm`: {`ContractConditions("@1AdminCondition")`},
		`ReadPerm`:      {`true`},
		`UpdatePerm`:    {`ContractAccess("@1EditParameter","@1AddAssignMember","@1DelAssignMember","@1Mint")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditColumn`
	form = url.Values{
		`Name`:       {`value`},
		`ReadPerm`:   {`true`},
		`TableName`:  {`parameters`},
		`UpdatePerm`: {`ContractAccess("@1EditParameter","@1AddAssignMember","@1DelAssignMember","@1Mint")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditTable`
	form = url.Values{
		`InsertPerm`:    {`ContractAccess("@1RolesCreate","@1RolesInstall","@1MintRolesInstall")`},
		`Name`:          {`roles`},
		`NewColumnPerm`: {`ContractConditions("@1AdminCondition")`},
		`ReadPerm`:      {`true`},
		`UpdatePerm`:    {`ContractAccess("@1RolesAccessManager","@1RolesDelete")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	rnd = `@1EditTable`
	form = url.Values{
		`InsertPerm`:    {`ContractAccess("@1RolesAssign","@1VotingDecisionCheck","@1RolesInstall","@1MintRolesInstall")`},
		`Name`:          {`roles_participants`},
		`NewColumnPerm`: {`ContractConditions("@1AdminCondition")`},
		`ReadPerm`:      {`true`},
		`UpdatePerm`:    {`ContractAccess("@1RolesUnassign")`},
	}
	assert.NoError(t, postTx(rnd, &form))

	//
	rnd = `@1EditAppParam`
	form = url.Values{
		`Conditions`: {`ContractConditions("@1DeveloperCondition")`},
		`Id`:         {"23"},
		`Value`:      {`@1vp_everybody,@1vp_manual,@1vp_role,@1vp_rolelist_all,@1vp_rolelist_one,@1vp_group,@1vp_pool`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)

}

func TestNewMineTotal1(t *testing.T) {
	//xlsx, err := excelize.OpenFile("/Users/scott/Desktop/mine_info-1585034424.xlsx")
	//if err != nil {
	//	panic(err.Error())
	//}
	//sheetName := xlsx.GetSheetName(1)
	//rows := xlsx.GetRows(sheetName)
	//for i := 1; i < len(rows); i++ {
	//	fmt.Println(rows[0][23], rows[0][24], rows[0][25])
	//}

	arr := []int64{1, 2, 3, 4, 4, 5, 5, 5}
	var repeatArr []int64
	repeatNumber := make(map[int64]bool)
	m1 := make(map[int64]int64)

	for _, i2 := range arr {
		if !repeatNumber[i2] {
			repeatNumber[i2] = true
			repeatArr = append(repeatArr, i2)
		}
	}
	for i := 0; i < len(repeatArr); i++ {
		if len(repeatArr) == 1 || i == 0 {
			m1[repeatArr[i]] = 0
		}
		if i < len(repeatArr)-1 {
			m1[repeatArr[i+1]] = repeatArr[i]
		}
	}
	fmt.Println(m1)
}

func TestNewMineTotal(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `NewMineInfo`
	form := url.Values{
		`Number`:          {`P9Mv0FeQ74`},
		`Name`:            {`mine 1`},
		`DevPubKey`:       {`6286e0aa474a67484e76994f11e99a59398a6cb4f39171eb3a8f162c58ee15b3`},
		`DevActivePubKey`: {`04feaa25415c17f0d105889e96cac430d4183a2f0d1fea5cdf09d90edba93083e8114f78e9657375daddff7ef71d60ab9e514ecb5994f6f892aebe7b1cbf30891a`},
		`Type`:            {`2`},
		`MaxCapacity`:     {`10`},
		`Capacity`:        {`1`},
		`MinCapacity`:     {`1`},
		`IP`:              {`127.0.0.1`},
		`Gps`:             {`11.12.365`},
		`Ver`:             {`1`},
		`Version`:         {`1.0`},
		`ValidTime`:       {`31622400`},
		`StartTime`:       {`1564627117`},
		`EndTime`:         {`1596249517`},
		`Location`:        {`China-ShenZhen`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
	rnd = `EditMineStatus`
	form = url.Values{
		`DevAddr`:       {`0874-3258-8928-9828-0009`},
		`Status`:        {`1`},
		`IP`:            {`127.0.0.1`},
		`Location`:      {`China-ShenZhen`},
		`CapacityTotal`: {`10048576`},
		`CapacityUsed`:  {`1024`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)

	rnd = `NewMineStakeRule`
	form = url.Values{
		`Capacity`:      {`100`},
		`AmountUnit`:    {`200`},
		`TimeUnit`:      {`300`},
		`TokenT`:        {`30000000000000000`},
		`Threshold`:     {`100000000000000`},
		`Type`:          {`2`},
		`Param1`:        {`10`},
		`Param2`:        {`20`},
		`InviterType`:   {`2`},
		`InviterParam1`: {`30`},
		`InviterParam2`: {`20`},
		`InviterDays`:   {`30`},
		`InviteeType`:   {`2`},
		`InviteeParam1`: {`60`},
		`InviteeParam2`: {`40`},
		`InviteeDays`:   {`90`},
		`Info`:          {`This is a simple rule info`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)

	rnd = `ReleaseMineStakeRule`
	form = url.Values{
		`RuleID`:  {`1`},
		`Release`: {`1`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)
}
func TestNewMineUser(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	var (
		signstr, pub, data string
		sign               []byte
	)
	data = "ACTIVATE"
	sign, err := crypto.SignString("77bb3d290c845a905a271ac50f44e18999394a8c9e5588c387bfe40ee39aa70d", data)
	assert.NoError(t, err)
	signstr = hex.EncodeToString(sign)
	pub, err = PrivateToPublicHex("77bb3d290c845a905a271ac50f44e18999394a8c9e5588c387bfe40ee39aa70d")
	assert.NoError(t, err)

	rnd := `ActiveMineInfo`
	form := url.Values{
		`Sign`:            {signstr},
		`Data`:            {data},
		`DevActivePubKey`: {pub},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)

	rnd = `NewMineInvite`
	form = url.Values{
		`BindAddr`: {`0874-3258-8928-9828-0009`},
		`Invite`:   {`Zr9F46pg`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)

	rnd = `NewMineStake`
	form = url.Values{
		`DevAddr`: {`0874-3258-8928-9828-0009`},
		`Cycle`:   {`4`},
		`Amount`:  {`2`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)
}
func TestBatchNewMineTotal(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	xlsx, err := excelize.OpenFile("/Users/scott/Desktop/mine_info-1585034424.xlsx")
	if err != nil {
		panic(err.Error())
	}
	sheetNameCloud := xlsx.GetSheetName(1)
	rowsCloud := xlsx.GetRows(sheetNameCloud)
	sheetNameHard := xlsx.GetSheetName(2)
	rowsHard := xlsx.GetRows(sheetNameHard)
	sheetNameUnion := xlsx.GetSheetName(3)
	rowsUnion := xlsx.GetRows(sheetNameUnion)
	var rows [][]string
	for i := 1; i < len(rowsCloud); i++ {
		rows = append(rows, rowsCloud[i])
	}
	for i := 1; i < len(rowsHard); i++ {
		rows = append(rows, rowsHard[i])
	}
	for i := 1; i < len(rowsUnion); i++ {
		rows = append(rows, rowsUnion[i])
	}
	for i := 0; i < len(rows); i++ {
		vt := converter.Int64ToStr(converter.StrToInt64(rows[i][19]) * 24 * 60 * 60)
		s20 := rows[i][20]
		if strings.Contains(s20, "/") {
			s20 = strings.ReplaceAll(s20, "/", "-")
		}

		st, _ := smart.UnixDateTimeLocation(s20, "")
		s21 := rows[i][21]
		if strings.Contains(s21, "/") {
			s21 = strings.ReplaceAll(s21, "/", "-")
		}
		et, _ := smart.UnixDateTimeLocation(s21, "")
		fmt.Println(rows[i][0], rows[i][1], rows[i][2])

		form := url.Values{
			`Number`:          {rows[i][2]},
			`Name`:            {rows[i][3]},
			`DevPubKey`:       {rows[i][4]},
			`DevActivePubKey`: {rows[i][6]},
			`Type`:            {rows[i][9]},
			`MaxCapacity`:     {rows[i][10]},
			`Capacity`:        {rows[i][11]},
			`MinCapacity`:     {rows[i][12]},
			`IP`:              {rows[i][14]},
			`Location`:        {rows[i][15]},
			`Gps`:             {rows[i][16]},
			`Ver`:             {rows[i][17]},
			`Version`:         {rows[i][18]},
			`Status`:        {`1`},
			`IP`:            {rows[i][14]},
			`Location`:      {rows[i][15]},
			`CapacityTotal`: {`10048576`},
			`CapacityUsed`:  {`1024`},
		}
		_, _, err = postTxResult(`EditMineStatus`, &form)
		assert.NoError(t, err)
	}

	rnd := `NewMineStakeRule`
	form := url.Values{
		`Capacity`:      {`1`},
		`IsInc`:         {`0`},
		`AmountUnit`:    {`10`},
		`AmountMin`:     {`1`},
		`AmountMax`:     {`20`},
		`AmountInc`:     {`3`},
		`TimeUnit`:      {`30`},
		`TimeMin`:       {`1`},
		`TimeMax`:       {`20`},
		`TimeInc`:       {`2`},
		`Type`:          {`2`},
		`Param1`:        {`10`},
		`Param2`:        {`20`},
		`InviterType`:   {`2`},
		`InviterParam1`: {`30`},
		`InviterParam2`: {`20`},
		`InviterDays`:   {`30`},
		`InviteeType`:   {`2`},
		`InviteeParam1`: {`60`},
		`InviteeParam2`: {`40`},
		`InviteeDays`:   {`90`},
		`Info`:          {`This is a simple rule info`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)

	rnd = `ReleaseMineStakeRule`
	form = url.Values{
		`RuleID`:  {`1`},
		`Release`: {`1`},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)
}
func TestBatchNewMineUserCloud(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	xlsx, err := excelize.OpenFile("/Users/scott/Desktop/mine_info-1585034424.xlsx")
	if err != nil {
		panic(err.Error())
	}
	sheetName := xlsx.GetSheetName(1)

	rows := xlsx.GetRows(sheetName)
	for i := 1; i < len(rows); i++ {
		fmt.Println(rows[i][0], rows[i][6], rows[i][7])
		var (
			signstr, data string
			sign          []byte
		)
		data = "ACTIVATE"
		sign, err := crypto.SignString(rows[i][7], data)
		assert.NoError(t, err)
		signstr = hex.EncodeToString(sign)
		assert.NoError(t, err)

		rnd := `ActiveMineInfo`
		form := url.Values{
			`Sign`:            {signstr},
			`Data`:            {data},
			`DevActivePubKey`: {rows[i][6]},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)

		rnd = `NewMineInvite`
		form = url.Values{
			`BindAddr`: {rows[i][1]},
			`Invite`:   {utils.RandNumber(8)},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)

		rnd = `NewMineStake`
		form = url.Values{
			`DevAddr`: {rows[i][1]},
			`Cycle`:   {converter.Int64ToStr(rand.Int63n(10))},
			`Amount`:  {converter.Int64ToStr(rand.Int63n(10))},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)
	}
}
func TestBatchNewMineUserHard(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	xlsx, err := excelize.OpenFile("/Users/scott/Desktop/mine_info-1585034424.xlsx")
	if err != nil {
		panic(err.Error())
	}
	sheetName := xlsx.GetSheetName(2)

	rows := xlsx.GetRows(sheetName)
	for i := 1; i < len(rows); i++ {
		fmt.Println(rows[i][0], rows[i][6], rows[i][7])
		var (
			signstr, data string
			sign          []byte
		)
		data = "ACTIVATE"
		sign, err := crypto.SignString(rows[i][7], data)
		assert.NoError(t, err)
		signstr = hex.EncodeToString(sign)
		assert.NoError(t, err)

		rnd := `ActiveMineInfo`
		form := url.Values{
			`Sign`:            {signstr},
			`Data`:            {data},
			`DevActivePubKey`: {rows[i][6]},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)

		rnd = `NewMineInvite`
		form = url.Values{
			`BindAddr`: {rows[i][1]},
			`Invite`:   {utils.RandNumber(8)},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)

		rnd = `NewMineStake`
		form = url.Values{
			`DevAddr`: {rows[i][1]},
			`Cycle`:   {converter.Int64ToStr(rand.Int63n(10))},
			`Amount`:  {converter.Int64ToStr(rand.Int63n(10))},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)
	}
}
func TestBatchNewMineUserUnion(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "4"))
	xlsx, err := excelize.OpenFile("/Users/scott/Desktop/mine_info-1585034424.xlsx")
	if err != nil {
		panic(err.Error())
	}
	sheetName := xlsx.GetSheetName(3)

	rows := xlsx.GetRows(sheetName)
	for i := 1; i < len(rows); i++ {
		fmt.Println(rows[i][0], rows[i][6], rows[i][7])
		var (
			signstr, data string
			sign          []byte
		)
		data = "ACTIVATE"
		sign, err := crypto.SignString(rows[i][7], data)
		assert.NoError(t, err)
		signstr = hex.EncodeToString(sign)
		assert.NoError(t, err)

		rnd := `ActiveMineInfo`
		form := url.Values{
			`Sign`:            {signstr},
			`Data`:            {data},
			`DevActivePubKey`: {rows[i][6]},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)

		rnd = `NewMineInvite`
		form = url.Values{
			`BindAddr`: {rows[i][1]},
			`Invite`:   {utils.RandNumber(8)},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)

		rnd = `NewMineStake`
		form = url.Values{
			`DevAddr`: {rows[i][1]},
			`Cycle`:   {converter.Int64ToStr(rand.Int63n(10))},
			`Amount`:  {converter.Int64ToStr(rand.Int63n(10))},
		}
		_, _, err = postTxResult(rnd, &form)
		assert.NoError(t, err)
	}
}

//user - up minepool file
func TestFilePoolUpload(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	rnd := `@1FilePoolUpload`
	data, err := ioutil.ReadFile("/Users/scott/Pictures/favicon.png")
	assert.NoError(t, err)

	file := make(map[interface{}]interface{})
	file["Body"] = data
	file["MimeType"] = `image/png`
	file["Name"] = `favicon.png`
	form := contractParams{
		`BufferKey`: `poollogo`,
		`FileData`:  file,
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//user - new pool request
func TestNewPoolRequest(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	rnd := `@1NewPoolRequest`
	form := url.Values{
		`Logo`:            {`1`},
		`Name`:            {`amod`},
		`SettleType`:      {`1`},
		`SettleRate`:      {`1`},
		`SettleMinAmount`: {`1000000`},
		`SettleCycle`:     {`12`},
		`WebUrl`:          {`http://a.b.c`},
		`PoolAddr`:        {`0454-4233-9004-4311-2470`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//manage - pool review
func TestPoolRequestDecision(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))

	rnd := `@1PoolRequestDecision`
	form := url.Values{
		`PRId`: {"1"},
		`Opt`:  {"accept"},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//miner - review fail to unstake
func TestRegainPoolStake(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1RegainPoolStake`
	form := url.Values{
		`PRId`:      {"1"},
		`Situation`: {"1"},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//user-join pool
func TestNewMinePoolGroup(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))

	rnd := `@1NewMinePoolGroup`
	form := url.Values{
		`DevAddr`:  {`0874-3258-8928-9828-0009`},
		`PoolAddr`: {`0454-4233-9004-4311-2470`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//user - leave pool
func TestDeleteMinePoolGroup(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))

	rnd := `@1DeleteMinePoolGroup`
	form := url.Values{
		`DevAddr`:   {`0874-3258-8928-9828-0009`},
		`PoolAddr`:  {`0454-4233-9004-4311-2470`},
		`Automatic`: {`2`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//pool manage - edit pool info
func TestEditPoolRequest(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1EditPoolRequest`
	form := url.Values{
		`PoolName`:        {`amod2`},
		`Type`:            {`1`},
		`SettleRate`:      {`2`},
		`SettleMinAmount`: {`11000000`},
		`SettleCycle`:     {`4`},
		`WebUrl`:          {`http://s.s.s`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//manage pool - review - en pool info
func TestTimeUpdatePool(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	rnd := `@1TimeUpdatePool`
	form := url.Values{
		`RequestId`: {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//vote - Agree to modify the mining pool information
func TestVotingDecisionAccept(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	rnd := `@1VotingDecisionAccept`
	form := url.Values{
		`VotingId`: {`1`},
		`RoleId`:   {`0`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//The original pool owner-initiate a request to transfer the pool
func TestSwitchPoolRequest(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	rnd := `@1SwitchPoolRequest`
	form := url.Values{
		`NewPoolerAddr`: {`0289-4067-9418-6057-9135`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//switch pool decision
func TestSwitchPoolDecision(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "4"))
	rnd := `@1SwitchPoolDecision`
	form := url.Values{
		`SwitchId`: {`1`},
		`Opt`:      {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//
//The original pool owner-the transfer is successful and the stake deposit amount is released
func TestRegainPoolStake1(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1RegainPoolStake`
	form := url.Values{
		`PRId`:       {"1"},
		`Situation`:  {"3"},
		`SwitchAddr`: {"0289-4067-9418-6057-9135"},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//pool to miner notifications
func TestPoolToMinerNotifications(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1PoolToMinerNotifications`
	form := url.Values{
		`Header`: {`hello`},
		`Body`:   {`xxxdatea`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//Mining Pool Owner-Announcement-Initiating the Dissolution of the Mining Pool
func TestDeletePoolRequest(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1DeletePoolRequest`
	form := url.Values{
		`PoolName`: {`amod`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//Mining Pool Owner-Announcement-Confirmation to disband the mining pool
func TestConfirmDeletePool(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1ConfirmDeletePool`
	form := url.Values{
		`PoolName`: {`amod`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//regainpoolstake
func TestRegainPoolStake2(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))

	rnd := `@1RegainPoolStake`
	form := url.Values{
		`PRId`:      {"1"},
		`Situation`: {"2"},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//new pool activity
func TestNewPoolActivity(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))

	rnd := `@1NewPoolActivity`
	form := url.Values{
		`Banner`:    {`1`},
		`Logo`:      {`1`},
		`Name`:      {`moode8`},
		`StartTime`: {`1591472988`},
		`EndTime`:   {`1690478988`},
		`Url`:       {`http://a.b.c`},
		`Status`:    {`2`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//edit pool activity
func TestEditPoolActivity(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))

	rnd := `@1EditPoolActivity`
	form := url.Values{
		`ActivityId`: {`2`},
		`Banner`:     {`1`},
		`Logo`:       {`1`},
		`Name`:       {`moode22`},
		`StartTime`:  {`1591472988`},
		`EndTime`:    {`1690478988`},
		`Url`:        {`http://a.b.c`},
		`Status`:     {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//delete pool activity
func TestDeletePoolActivity(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))

	rnd := `@1DeletePoolActivity`
	form := url.Values{
		`ActivityId`: {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//queue pool activity
func TestQueuePoolActivity(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))

	rnd := `@1QueuePoolActivity`
	form := url.Values{
		`QueueId`:    {`1`},
		`NewQueueId`: {`3`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//new mine info
func TestNewMineInfo(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `NewMineInfo`
	form := url.Values{
		`Number`:          {`P9Mv0FeQ74`},
		`Name`:            {`mine 1`},
		`DevPubKey`:       {`6286e0aa474a67484e76994f11e99a59398a6cb4f39171eb3a8f162c58ee15b3`},
		`DevActivePubKey`: {`04feaa25415c17f0d105889e96cac430d4183a2f0d1fea5cdf09d90edba93083e8114f78e9657375daddff7ef71d60ab9e514ecb5994f6f892aebe7b1cbf30891a`},
		`Type`:            {`2`},
		`MaxCapacity`:     {`10`},
		`Capacity`:        {`1`},
		`MinCapacity`:     {`1`},
		`IP`:              {`127.0.0.1`},
		`Gps`:             {`11.12.365`},
		`Ver`:             {`1`},
		`Version`:         {`1.0`},
		`ValidTime`:       {`31622400`},
		`StartTime`:       {`1564627117`},
		`EndTime`:         {`1596249517`},
		`Location`:        {`China-ShenZhen`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//edit mine status
func TestEditMineStatus(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `EditMineStatus`
	form := url.Values{
		`DevAddr`:       {`0874-3258-8928-9828-0009`},
		`Status`:        {`1`},
		`IP`:            {`127.0.0.1`},
		`Location`:      {`China-ShenZhen`},
		`CapacityTotal`: {`10048576`},
		`CapacityUsed`:  {`1024`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//active mine not invite
func TestActiveMineInfo(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	var (
		signstr, pub, data string
		sign               []byte
	)
	data = "ACTIVATE"
	sign, err := crypto.SignString("77bb3d290c845a905a271ac50f44e18999394a8c9e5588c387bfe40ee39aa70d", data)
	assert.NoError(t, err)
	signstr = hex.EncodeToString(sign)
	pub, err = PrivateToPublicHex("77bb3d290c845a905a271ac50f44e18999394a8c9e5588c387bfe40ee39aa70d")
	assert.NoError(t, err)

	rnd := `ActiveMineInfo`
	form := url.Values{
		`Sign`:            {signstr},
		`Data`:            {data},
		`DevActivePubKey`: {pub},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//new mine invite
func TestNewMineInvite(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	rnd := `NewMineInvite`
	form := url.Values{
		`BindAddr`: {`0874-3258-8928-9828-0009`},
		`Invite`:   {`Zr9F46pg`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//new mine info
func TestNewMineInfo1(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `NewMineInfo`
	form := url.Values{
		`Number`:          {`P9Mv0FeQ73`},
		`Name`:            {`mine 2`},
		`DevPubKey`:       {`f9f65079cf1e2048c7c300a63858483a8a0973e9ae185d8c34d6781a0d4e219a`},
		`DevActivePubKey`: {`04e201f481871c10a3617cefe2a0e70b94d696edf3a4d493d84ec9f5f275bde181894c7db7173e7b68467d406b65951cf8df6a7775dea3d20064480547a0a395fd`},
		`Type`:            {`2`},
		`MaxCapacity`:     {`10`},
		`Capacity`:        {`3`},
		`MinCapacity`:     {`1`},
		`IP`:              {`127.0.0.1`},
		`Gps`:             {`11.12.365`},
		`Ver`:             {`1`},
		`Version`:         {`1.0`},
		`StartTime`:       {`1564627117`},
		`EndTime`:         {`1596249517`},
		`Location`:        {`Singapore`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//edit new mine status
func TestEditMineStatus1(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `EditMineStatus`
	form := url.Values{
		`DevAddr`:       {`0354-3002-5250-5835-2444`},
		`Status`:        {`1`},
		`IP`:            {`127.0.0.1`},
		`Location`:      {`Singapore`},
		`CapacityTotal`: {`3145728`},
		`CapacityUsed`:  {`1024`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//new stake rule
func TestNewMineStakeRule(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `NewMineStakeRule`
	form := url.Values{
		`Capacity`:      {`1`},
		`AmountUnit`:    {`1000`},
		`AmountMin`:     {`3`},
		`AmountMax`:     {`10`},
		`AmountInc`:     {`3`},
		`TimeUnit`:      {`30`},
		`TimeMin`:       {`2`},
		`TimeMax`:       {`10`},
		`TimeInc`:       {`2`},
		`Type`:          {`3`},
		`Param1`:        {`30`},
		`Param2`:        {`20`},
		`Param3`:        {`99999`},
		`InviterType`:   {`2`},
		`InviterParam1`: {`30`},
		`InviterParam2`: {`20`},
		`InviterDays`:   {`30`},
		`InviteeType`:   {`2`},
		`InviteeParam1`: {`60`},
		`InviteeParam2`: {`40`},
		`InviteeDays`:   {`90`},
		`Info`:          {`This is a simple rule info`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//release mine stake rule
func TestReleaseMineStakeRule(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `ReleaseMineStakeRule`
	form := url.Values{
		`RuleID`:  {`1`},
		`Release`: {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//activemine invite
func TestActiveMineInfo1(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	var (
		signstr, pub, data, invite string
		sign                       []byte
	)
	data = "ACTIVATE"
	invite = "Zr9F46pg"
	sign, err := crypto.SignString("20c7fe293d53d5546926348745b7fcc8a94796aa06b44590a30268d4c1039b08", data)
	assert.NoError(t, err)
	signstr = hex.EncodeToString(sign)
	pub, err = PrivateToPublicHex("20c7fe293d53d5546926348745b7fcc8a94796aa06b44590a30268d4c1039b08")
	assert.NoError(t, err)

	rnd := `ActiveMineInfo`
	form := url.Values{
		`Sign`:            {signstr},
		`Data`:            {data},
		`DevActivePubKey`: {pub},
		`Invite`:          {invite},
	}
	_, _, err = postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//mine stake
func TestNewMineStake(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	rnd := `NewMineStake`
	form := url.Values{
		`DevAddr`: {`0874-3258-8928-9828-0009`},
		`Cycle`:   {`4`},
		`Amount`:  {`5`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//mine stake
func TestNewMineStake1(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "3"))
	rnd := `NewMineStake`
	form := url.Values{
		`DevAddr`: {`0354-3002-5250-5835-2444`},
		`Cycle`:   {`2`},
		`Amount`:  {`4`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//import mine
func TestMineImport(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	form := url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	if err := postTx(`MoneyTransfer`, &form); err != nil {
		t.Error(err)
		return
	}
}

//review mine stake rule
func TestReviewMineStakeRule(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `ReviewMineStakeRule`
	form := url.Values{
		`RuleID`:          {`1`},
		`ExpiredReview`:   {`1`},
		`UnexpiredReview`: {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//mier switch
func TestSwitchMineOwnerForKeyID(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	rnd := `SwitchMineOwnerForKeyID`
	form := url.Values{
		`DevAddr`:    {`0874-3258-8928-9828-0009`},
		`NewKeyAddr`: {`1448-9912-4891-3624-0588`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//withdrawmine stake
func TestWithdrawMineStake(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "2"))
	rnd := `WithdrawMineStake`
	form := url.Values{
		`StakeID`: {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//review mine stake
func TestReviewMineStake(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `ReviewMineStake`
	form := url.Values{
		`ReviewID`: {`1`},
		`Review`:   {`2`},
		`Reason`:   {`pass`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//delete mine stake rule
func TestDeleteMineStakeRule(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `DeleteMineStakeRule`
	form := url.Values{
		`RuleID`: {`1`},
	}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

//update stakeexpired
func TestUpdateStakeExpired(t *testing.T) {
	assert.NoError(t, keyLoginex(1, "1"))
	rnd := `UpdateStakeExpired`
	form := url.Values{}
	_, _, err := postTxResult(rnd, &form)
	assert.NoError(t, err)
}

func TestTxtype(t *testing.T) {

	i := int64(123)
	d := int8(i)

	// 8 hour
	hh, _ := time.ParseDuration("1h")
	hh1 := time.Now().Add(hh)
	fmt.Println(i)
	fmt.Println(d)
	fmt.Println(hh1)
	//assert.NoError(t, keyLoginex(1, "1"))
	//rnd := `DeleteMineStakeRule`
	//form := url.Values{
	//	`RuleID`: {`1`},
	//}
	//_, _, err := postTxResult(rnd, &form)
	//assert.NoError(t, err)
}

//new miner
func TestNewMinerDataS(t *testing.T) {
	st, err := miner.MakeMiningPoolData(10000000)
	//st,err :=miner.MakeMiningPoolData(100000000)
	t1 := time.Now()
	dl := rand.Intn(st)
	k := miner.GetMiner(dl)
	fmt.Println(t1)
	t2 := time.Now()
	fmt.Println(t2.Sub(t1))
	fmt.Println(time.Since(t1))
	//dl1 := rand.Intn(st)
	//k1 :=miner.GetMiner(dl1)
	//dl2 := rand.Intn(st)
	//k2 :=miner.GetMiner(dl2)
	//dl3 := rand.Intn(st)
	//k3 :=miner.GetMiner(dl3)
	//dl4 := rand.Intn(st)
	//k4 :=miner.GetMiner(dl4)
	fmt.Println(k)
	//fmt.Println(k1)
	//fmt.Println(k2)
	//fmt.Println(k3)
	//fmt.Println(k4)
	assert.NoError(t, err)
}

func TestMoneyTokenSend(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	form := url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	if err := postTx(`TokensSend`, &form); err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Amount`: {`2440000`}, `Recipient`: {`1109-7770-3360-6764-7059`}, `Comment`: {`Test`}}
	if err := postTx(`TokensSend`, &form); err != nil {
		t.Error(err)
		return
	}
	//form = url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005207000`}}
	//if err := postTx(`TokensSend`, &form); cutErr(err) != `{"type":"error","error":"Recipient 0005207000 is invalid"}` {
	//	t.Error(err)
	//	return
	//}
	size := 1000000
	big := make([]byte, size)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < size; i++ {
		big[i] = '0' + byte(rand.Intn(10))
	}
	//form = url.Values{`Amount`: {string(big)}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	//if err := postTx(`TokensSend`, &form); err.Error() != `400 {"error": "E_LIMITFORSIGN", "msg": "Length of forsign is too big (1000106)" , "params": ["1000106"]}` {
	//	t.Error(err)
	//	return
	//}
}

func TestMoneySignTokenSend(t *testing.T) {
	if err := keyLogin(1); err != nil {
		t.Error(err)
		return
	}

	form := url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	if err := postSignTx(`TokensSend`, &form); err != nil {
		t.Error(err)
		return
	}
	form = url.Values{`Amount`: {`2440000`}, `Recipient`: {`1109-7770-3360-6764-7059`}, `Comment`: {`Test`}}
	if err := postSignTx(`TokensSend`, &form); err != nil {
		t.Error(err)
		return
	}
	//form = url.Values{`Amount`: {`53330000`}, `Recipient`: {`0005207000`}}
	//if err := postTx(`TokensSend`, &form); cutErr(err) != `{"type":"error","error":"Recipient 0005207000 is invalid"}` {
	//	t.Error(err)
	//	return
	//}
	//size := 1000000
	//big := make([]byte, size)
	//rand.Seed(time.Now().UnixNano())
	//for i := 0; i < size; i++ {
	//	big[i] = '0' + byte(rand.Intn(10))
	//}
	//form = url.Values{`Amount`: {string(big)}, `Recipient`: {`0005-2070-2000-0006-0200`}}
	//if err := postTx(`TokensSend`, &form); err.Error() != `400 {"error": "E_LIMITFORSIGN", "msg": "Length of forsign is too big (1000106)" , "params": ["1000106"]}` {
	//	t.Error(err)
	//	return
	//}
}

//token refresh test
func TestTokenBalance(t *testing.T) {
	if err := keyLoginToken(1); err != nil {
		t.Error(err)
		return
	}
	var ret balanceResult
	err := sendGet(`balance/`+gAddress, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("balance ok")
	time.Sleep(3 * time.Second)

	for i := 0; i < 5; i++ {
		err = sendGet(`balance/`+gAddress, nil, &ret)
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Println("balance ok ", i)

		time.Sleep(3 * time.Second)
	}

	time.Sleep(30 * time.Second)
	err = sendGet(`balance/`+gAddress, nil, &ret)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("balance 31 ok")
}
