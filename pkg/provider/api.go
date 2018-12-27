package provider

type Provider struct {
	Database Database
	Backup   Backup
}

func init() (*Provider, error) {

}
