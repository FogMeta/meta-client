package subcommand

import (
	"fmt"
	"strings"
	"time"

	"go-swan-client/logs"
	"go-swan-client/model"

	"go-swan-client/common/client"
	"go-swan-client/common/constants"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func CreateTask(confTask *model.ConfTask, inputDir string, taskName, outputDir, minerFid, dataset, description *string) (*string, error) {
	err := CheckInputDir(inputDir)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	outputDir, err = CreateOutputDir(outputDir)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	logs.GetLogger().Info("you output dir: ", *outputDir)

	if !confTask.PublicDeal && (minerFid == nil || len(*minerFid) == 0) {
		err := fmt.Errorf("please provide -miner for non public deal")
		logs.GetLogger().Error(err)
		return nil, err
	}
	if confTask.BidMode == constants.TASK_BID_MODE_AUTO && minerFid != nil && len(*minerFid) != 0 {
		logs.GetLogger().Warn("-miner is unnecessary for aubo-bid task, it will be ignored")
	}

	if taskName == nil || len(*taskName) == 0 {
		nowStr := "task_" + time.Now().Format("2006-01-02_15:04:05")
		taskName = &nowStr
	}

	maxPrice, err := decimal.NewFromString(confTask.MaxPrice)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}
	//generateMd5 := config.GetConfig().Sender.GenerateMd5

	logs.GetLogger().Info("task settings:")
	logs.GetLogger().Info("public task: ", confTask.PublicDeal)
	logs.GetLogger().Info("verified deals: ", confTask.VerifiedDeal)
	logs.GetLogger().Info("connected to swan: ", !confTask.OfflineMode)
	logs.GetLogger().Info("fastRetrieval: ", confTask.FastRetrieval)

	carFiles := ReadCarFilesFromJsonFile(inputDir, constants.JSON_FILE_NAME_BY_UPLOAD)
	if carFiles == nil {
		err := fmt.Errorf("failed to read car files from :%s", inputDir)
		logs.GetLogger().Error(err)
		return nil, err
	}

	isPublic := 0
	if confTask.PublicDeal {
		isPublic = 1
	}

	taskType := constants.TASK_TYPE_REGULAR
	if confTask.VerifiedDeal {
		taskType = constants.TASK_TYPE_VERIFIED
	}

	task := model.Task{
		TaskName:          *taskName,
		CuratedDataset:    *dataset,
		Description:       *description,
		FastRetrievalBool: confTask.FastRetrieval,
		Type:              &taskType,
		IsPublic:          &isPublic,
		MaxPrice:          &maxPrice,
		BidMode:           &confTask.BidMode,
		ExpireDays:        &confTask.ExpireDays,
		MinerFid:          minerFid,
		Uuid:              uuid.NewString(),
	}

	for _, carFile := range carFiles {
		carFile.Uuid = task.Uuid
		carFile.MinerFid = task.MinerFid

		if confTask.StorageServerType == constants.STORAGE_SERVER_TYPE_WEB_SERVER {
			carFile.CarFileUrl = strings.TrimRight(confTask.WebServerDownloadUrlPrefix, "/") + "/" + carFile.CarFileName
		}
	}

	if !confTask.PublicDeal {
		_, err := SendDeals2Miner(nil, *taskName, *minerFid, *outputDir, carFiles)
		if err != nil {
			return nil, err
		}
	}

	jsonFileName := *taskName + constants.JSON_FILE_NAME_BY_TASK
	csvFileName := *taskName + constants.CSV_FILE_NAME_BY_TASK
	err = WriteCarFilesToFiles(carFiles, *outputDir, jsonFileName, csvFileName)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	err = SendTask2Swan(confTask, task, carFiles, *outputDir)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	return &jsonFileName, nil
}

func SendTask2Swan(confTask *model.ConfTask, task model.Task, carFiles []*model.FileDesc, outDir string) error {
	csvFilename := task.TaskName + ".csv"
	csvFilePath, err := CreateCsv4TaskDeal(carFiles, outDir, csvFilename)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	if confTask.OfflineMode {
		logs.GetLogger().Info("Working in Offline Mode. You need to manually send out task on filwan.com.")
		return nil
	}

	logs.GetLogger().Info("Working in Online Mode. A swan task will be created on the filwan.com after process done. ")
	swanClient, err := client.SwanGetClient(confTask.SwanApiUrl, confTask.SwanApiKey, confTask.SwanAccessToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	swanCreateTaskResponse, err := swanClient.SwanCreateTask(task, csvFilePath)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	if swanCreateTaskResponse.Status != "success" {
		err := fmt.Errorf("error, status%s, message:%s", swanCreateTaskResponse.Status, swanCreateTaskResponse.Message)
		logs.GetLogger().Info(err)
		return err
	}

	logs.GetLogger().Info("status:", swanCreateTaskResponse.Status, ", message:", swanCreateTaskResponse.Message)

	return nil
}
