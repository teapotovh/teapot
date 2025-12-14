package ldap

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// User is an abstracted view over a user entry in LDAP.
type User struct {
	DN        string
	Username  string
	Firstname string
	Lastname  string
	Mail      string
	Home      string
	Groups    []string
	Accesses  []string
	UID       int
	GID       int
	Admin     bool
}

func filterDN(dns []string, suffix string) (result []string) {
	for _, dn := range dns {
		if strings.HasSuffix(dn, suffix) {
			result = append(result, dn)
		}
	}

	return result
}

func (c *Client) mapUser(entry *ldap.Entry) (*User, error) {
	dn := entry.DN
	username := entry.GetAttributeValue("cn")
	firstname := entry.GetAttributeValue("givenname")
	lastname := entry.GetAttributeValue("sn")
	mail := entry.GetAttributeValue("mail")
	rawUID := entry.GetAttributeValue("uidnumber")

	uid, err := strconv.Atoi(rawUID)
	if err != nil {
		return nil, fmt.Errorf("error while parsing uid: %w", err)
	}

	rawGID := entry.GetAttributeValue("gidnumber")

	gid, err := strconv.Atoi(rawGID)
	if err != nil {
		return nil, fmt.Errorf("error while parsing gid: %w", err)
	}

	home := entry.GetAttributeValue("homedirectory")

	memberof := entry.GetAttributeValues("memberof")
	admin := slices.Contains(memberof, c.adminGroupDN)
	groups := filterDN(memberof, c.groupsDN)
	accesses := filterDN(memberof, c.accessesDN)

	return &User{
		DN:        dn,
		Username:  username,
		Firstname: firstname,
		Lastname:  lastname,
		Mail:      mail,
		UID:       uid,
		GID:       gid,
		Home:      home,

		Groups:   groups,
		Accesses: accesses,
		Admin:    admin,
	}, nil
}

func (c *Client) Users() ([]*User, error) {
	entries, err := c.list()
	if err != nil {
		return nil, fmt.Errorf("error while listing all users: %w", err)
	}

	var users []*User

	for _, entry := range entries {
		user, err := c.mapUser(entry)
		if err != nil {
			username := entry.GetAttributeValue("cn")
			return nil, fmt.Errorf("error while mapping user %s: %w", username, err)
		}

		users = append(users, user)
	}

	return users, nil
}

func (c *Client) User(username string) (*User, error) {
	entry, err := c.find(username)
	if err != nil {
		return nil, fmt.Errorf("error while looking up user: %w", err)
	}

	return c.mapUser(entry)
}
