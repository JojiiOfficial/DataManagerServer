package models

import (
	"errors"

	"github.com/JojiiOfficial/gaw"
	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
)

// User user in db
type User struct {
	gorm.Model
	Username string
	Password string
	RoleID   uint  `sql:"index"`
	Role     *Role `gorm:"association_autoupdate:false;association_autocreate:false"`
}

// Login login user
func (user *User) Login(db *gorm.DB, machineID string) (*LoginSession, error) {
	// Truncat machineID if too big
	if len(machineID) > 100 {
		machineID = ""
	}

	// Return if user not exists
	if has, err := user.Has(db, true); !has {
		return nil, err
	}

	// Clean old sessions for user + machineID
	if err := user.cleanOldSessions(db, machineID); err != nil {
		logrus.Error(err)
	}

	// Generate session
	session := NewSession(user, machineID)
	if session == nil {
		return nil, errors.New("Can't generate session")
	}

	// Save session
	if err := db.Create(&session).Error; err != nil {
		return nil, err
	}

	return session, nil
}

func (user *User) cleanOldSessions(db *gorm.DB, machineID string) error {
	if len(machineID) == 0 {
		return nil
	}

	// Delete session(s)
	return db.Unscoped().Where(&LoginSession{
		UserID:    user.ID,
		MachineID: machineID,
	}).Delete(&LoginSession{}).Error
}

// Register register user
func (user User) Register(db *gorm.DB, config *Config) error {
	// Return if user already exists
	has, _ := user.Has(db, false)
	if has {
		return ErrorUserAlreadyExists
	}

	user = User{
		Password: gaw.SHA512(user.Username + user.Password),
		Username: user.Username,
		RoleID:   config.GetDefaultRole().ID,
		Role:     config.GetDefaultRole(),
	}

	err := db.Create(&user).Error
	if err != nil {
		return err
	}

	// Create namespace for user
	_, err = user.CreateDefaultNamespace(db)
	return err
}

//Has return true if user exists
func (user *User) Has(db *gorm.DB, checkPass bool) (bool, error) {
	pass := ""
	if checkPass {
		pass = user.Password
	}
	//Check if user exists
	if err := db.Where(&User{
		Username: user.Username,
		Password: pass,
	}).First(user).Error; err != nil {
		return false, err
	}

	return true, nil
}

//GetDefaultNamespaceName return the name of the default namespace for a user
func (user *User) GetDefaultNamespaceName() string {
	return user.Username + "_default"
}

//CreateDefaultNamespace creates user namespace
func (user *User) CreateDefaultNamespace(db *gorm.DB) (*Namespace, error) {
	namespace := Namespace{
		Name: user.GetDefaultNamespaceName(),
		User: user,
	}

	//Crreate
	err := db.Create(&namespace).Error
	if err != nil {
		return nil, err
	}

	return &namespace, nil
}

//HasAccess return true if user has access to the given namespace
func (user *User) HasAccess(namespace *Namespace) bool {
	// User has access if it's his namespace or if he can write others
	return namespace.IsOwnedBy(user) || user.CanWriteForeignNamespace()
}
