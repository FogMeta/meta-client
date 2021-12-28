package command

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/filswan/go-swan-client/config"

	"github.com/filswan/go-swan-lib/logs"
	"github.com/filswan/go-swan-lib/utils"

	"github.com/filswan/go-swan-lib/client/lotus"
	"github.com/filswan/go-swan-lib/client/swan"
	libconstants "github.com/filswan/go-swan-lib/constants"
	libmodel "github.com/filswan/go-swan-lib/model"
)

type CmdAutoBidDeal struct {
	SwanApiUrl             string //required
	SwanApiKey             string //required when SwanJwtToken is not provided
	SwanAccessToken        string //required when SwanJwtToken is not provided
	SwanToken              string //required when SwanApiKey and SwanAccessToken are not provided
	LotusClientApiUrl      string //required
	LotusClientAccessToken string //required
	SenderWallet           string //required
	OutputDir              string //required
	DealSourceIds          []int  //required
}

func GetCmdAutoDeal(outputDir *string, minerFids string, metadataJsonPath string) *CmdAutoBidDeal {
	cmdAutoBidDeal := &CmdAutoBidDeal{
		SwanApiUrl:             config.GetConfig().Main.SwanApiUrl,
		SwanApiKey:             config.GetConfig().Main.SwanApiKey,
		SwanAccessToken:        config.GetConfig().Main.SwanAccessToken,
		LotusClientApiUrl:      config.GetConfig().Lotus.ClientApiUrl,
		LotusClientAccessToken: config.GetConfig().Lotus.ClientAccessToken,
		SenderWallet:           config.GetConfig().Sender.Wallet,
		OutputDir:              filepath.Join(config.GetConfig().Sender.OutputDir, time.Now().Format("2006-01-02_15:04:05")),
	}

	cmdAutoBidDeal.DealSourceIds = append(cmdAutoBidDeal.DealSourceIds, libconstants.TASK_SOURCE_ID_SWAN)
	cmdAutoBidDeal.DealSourceIds = append(cmdAutoBidDeal.DealSourceIds, libconstants.TASK_SOURCE_ID_SWAN_CLIENT)

	if !utils.IsStrEmpty(outputDir) {
		cmdAutoBidDeal.OutputDir = *outputDir
	}

	return cmdAutoBidDeal
}

func SendAutoBidDealsLoopByConfig(outputDir string) error {
	cmdAutoBidDeal := GetCmdAutoDeal(&outputDir, "", "")
	err := cmdAutoBidDeal.SendAutoBidDealsLoop()
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	return nil
}

func (cmdAutoBidDeal *CmdAutoBidDeal) SendAutoBidDealsLoop() error {
	err := utils.CreateDirIfNotExists(cmdAutoBidDeal.OutputDir, DIR_NAME_OUTPUT)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	for {
		err := cmdAutoBidDeal.SendAutoBidDeals()
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}

		time.Sleep(time.Second * 30)
	}
}

func (cmdAutoBidDeal *CmdAutoBidDeal) SendAutoBidDeals() error {
	for _, sourceId := range cmdAutoBidDeal.DealSourceIds {
		err := cmdAutoBidDeal.sendAutoBidDealsBySourceId(sourceId)
		if err != nil {
			logs.GetLogger().Error(err)
			return err
		}
	}

	return nil
}

func (cmdAutoBidDeal *CmdAutoBidDeal) SendAutoBidDealsByTaskUuid(taskUuid string) error {
	swanClient, err := swan.GetClient(cmdAutoBidDeal.SwanApiUrl, cmdAutoBidDeal.SwanApiKey, cmdAutoBidDeal.SwanAccessToken, cmdAutoBidDeal.SwanToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	params := swan.GetOfflineDealsByStatusParams{
		DealStatus: libconstants.OFFLINE_DEAL_STATUS_ASSIGNED,
		TaskUuid:   &taskUuid,
	}
	assignedOfflineDeals, err := swanClient.GetOfflineDealsByStatus(params)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	err = cmdAutoBidDeal.sendAutoBidDealsInArray(assignedOfflineDeals)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	return nil
}

func (cmdAutoBidDeal *CmdAutoBidDeal) sendAutoBidDealsBySourceId(sourceId int) error {
	swanClient, err := swan.GetClient(cmdAutoBidDeal.SwanApiUrl, cmdAutoBidDeal.SwanApiKey, cmdAutoBidDeal.SwanAccessToken, cmdAutoBidDeal.SwanToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	params := swan.GetOfflineDealsByStatusParams{
		DealStatus: libconstants.OFFLINE_DEAL_STATUS_ASSIGNED,
		SourceId:   &sourceId,
	}
	assignedOfflineDeals, err := swanClient.GetOfflineDealsByStatus(params)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	err = cmdAutoBidDeal.sendAutoBidDealsInArray(assignedOfflineDeals)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	return nil
}

func (cmdAutoBidDeal *CmdAutoBidDeal) sendAutoBidDealsInArray(assignedOfflineDeals []*libmodel.OfflineDeal) error {
	if len(assignedOfflineDeals) == 0 {
		logs.GetLogger().Info("no offline deals to be sent")
		return nil
	}

	swanClient, err := swan.GetClient(cmdAutoBidDeal.SwanApiUrl, cmdAutoBidDeal.SwanApiKey, cmdAutoBidDeal.SwanAccessToken, cmdAutoBidDeal.SwanToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	for _, assignedOfflineDeal := range assignedOfflineDeals {
		updateOfflineDealParams, err := cmdAutoBidDeal.sendAutobidDeal(assignedOfflineDeal)
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}

		if updateOfflineDealParams == nil {
			continue
		}
		err = swanClient.UpdateOfflineDeal(*updateOfflineDealParams)
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}
	}

	return nil
}

func (cmdAutoBidDeal *CmdAutoBidDeal) sendAutobidDeal(offlineDeal *libmodel.OfflineDeal) (*swan.UpdateOfflineDealParams, error) {
	offlineDeal.DealCid = strings.Trim(offlineDeal.DealCid, " ")
	if len(offlineDeal.DealCid) != 0 {
		logs.GetLogger().Info("deal already be sent, task:%s, deal:%d", *offlineDeal.TaskUuid, offlineDeal.Id)
		return nil, nil
	}

	if offlineDeal.TaskType == nil {
		err := fmt.Errorf("task type is nil")
		logs.GetLogger().Error(err)
		return nil, err
	}

	if offlineDeal.FastRetrieval == nil {
		err := fmt.Errorf("FastRetrieval is nil")
		logs.GetLogger().Error(err)
		return nil, err
	}

	if offlineDeal.MaxPrice == nil {
		err := fmt.Errorf("MaxPrice is nil")
		logs.GetLogger().Error(err)
		return nil, err
	}

	if offlineDeal.Duration == nil {
		err := fmt.Errorf("duration is nil")
		logs.GetLogger().Error(err)
		return nil, err
	}

	dealConfig := libmodel.DealConfig{
		VerifiedDeal:     *offlineDeal.TaskType == libconstants.TASK_TYPE_VERIFIED,
		FastRetrieval:    *offlineDeal.FastRetrieval == libconstants.TASK_FAST_RETRIEVAL_YES,
		SkipConfirmation: true,
		MaxPrice:         *offlineDeal.MaxPrice,
		StartEpoch:       int64(offlineDeal.StartEpoch),
		MinerFid:         offlineDeal.MinerFid,
		SenderWallet:     cmdAutoBidDeal.SenderWallet,
		Duration:         int(*offlineDeal.Duration),
		TransferType:     libconstants.LOTUS_TRANSFER_TYPE_MANUAL,
		PayloadCid:       offlineDeal.PayloadCid,
		PieceCid:         offlineDeal.PieceCid,
		FileSize:         offlineDeal.CarFileSize,
	}

	msg := fmt.Sprintf("send deal for task:%d,%s, deal:%d", offlineDeal.TaskId, *offlineDeal.TaskUuid, offlineDeal.Id)
	logs.GetLogger().Info(msg)

	lotusClient, err := lotus.LotusGetClient(cmdAutoBidDeal.LotusClientApiUrl, cmdAutoBidDeal.LotusClientAccessToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	for i := 0; i < 60; i++ {
		//change start epoch to ensure that the deal cid is different
		dealConfig.StartEpoch = dealConfig.StartEpoch - (int64)(i)

		dealCid, err := lotusClient.LotusClientStartDeal(&dealConfig)
		if err != nil {
			logs.GetLogger().Error("tried ", i+1, " times,", err)

			if strings.Contains(err.Error(), "already tracking identifier") {
				continue
			} else {
				break
			}
		}
		if dealCid == nil {
			logs.GetLogger().Info("no deal CID returned")
			continue
		}

		dealInfo := &libmodel.DealInfo{
			MinerFid:   offlineDeal.MinerFid,
			DealCid:    *dealCid,
			StartEpoch: int(dealConfig.StartEpoch),
		}

		updateOfflineDealParams := swan.UpdateOfflineDealParams{
			DealId:     offlineDeal.Id,
			DealCid:    dealCid,
			Status:     libconstants.OFFLINE_DEAL_STATUS_CREATED,
			StartEpoch: &dealInfo.StartEpoch,
		}

		logs.GetLogger().Info("deal sent successfully, task:", offlineDeal.TaskId, ", uuid:", *offlineDeal.TaskUuid, ", deal:", offlineDeal.Id, ", deal CID:", dealInfo.DealCid, ", start epoch:", dealInfo.StartEpoch, ", miner:", dealInfo.MinerFid)

		return &updateOfflineDealParams, nil
	}

	err = fmt.Errorf("failed to send deal")
	logs.GetLogger().Error(err)
	return nil, err
}