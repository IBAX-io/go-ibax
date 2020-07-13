	"github.com/IBAX-io/go-ibax/packages/converter"

	log "github.com/sirupsen/logrus"
)

// KillPid is killing process by PID
func KillPid(pid string) error {
	err := syscall.Kill(converter.StrToInt(pid), syscall.SIGTERM)
	if err != nil {
		log.WithFields(log.Fields{"pid": pid, "signal": syscall.SIGTERM}).Error("Error killing process with pid")
		return err
	}
	return nil
}
