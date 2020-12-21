
	log "github.com/sirupsen/logrus"
)

func loadContractTasks() error {
	stateIDs, _, err := model.GetAllSystemStatesIDs()
	if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("get all system states ids")
		return err
	}

	for _, stateID := range stateIDs {
		if !model.IsTable(fmt.Sprintf("%d_cron", stateID)) {
			return nil
		}

		c := model.Cron{}
		c.SetTablePrefix(fmt.Sprintf("%d", stateID))
		tasks, err := c.GetAllCronTasks()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get all cron tasks")
			return err
		}

		for _, cronTask := range tasks {
			err = scheduler.UpdateTask(&scheduler.Task{
				ID:       cronTask.UID(),
				CronSpec: cronTask.Cron,
				Handler: &contract.ContractHandler{
					Contract: cronTask.Contract,
				},
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Scheduler starts contracts on schedule
func Scheduler(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	d.sleepTime = time.Hour
	return loadContractTasks()
}
