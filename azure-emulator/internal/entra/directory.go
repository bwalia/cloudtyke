// Package entra holds the emulated Entra ID directory: tenants, users,
// groups, applications and service principals, plus credential checks used
// by the token service. Objects are stored as Microsoft Graph JSON so the
// Graph API can return them with minimal translation. Fields whose name
// starts with "_" are internal (relations, secret hashes) and are never
// returned by Graph.
package entra

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// Object is a directory object in Graph JSON form.
type Object = map[string]any

// Collections (Graph entity sets) persisted by the directory.
const (
	Users             = "users"
	Groups            = "groups"
	Applications      = "applications"
	ServicePrincipals = "servicePrincipals"
)

// AllCollections lists the directory object collections.
var AllCollections = []string{Users, Groups, Applications, ServicePrincipals}

// ODataType returns the @odata.type for a collection.
func ODataType(coll string) string {
	switch coll {
	case Users:
		return "#microsoft.graph.user"
	case Groups:
		return "#microsoft.graph.group"
	case Applications:
		return "#microsoft.graph.application"
	case ServicePrincipals:
		return "#microsoft.graph.servicePrincipal"
	case "directoryRoles":
		return "#microsoft.graph.directoryRole"
	case "oauth2PermissionGrants":
		return "#microsoft.graph.oAuth2PermissionGrant"
	}
	return "#microsoft.graph.directoryObject"
}

// Tenant is an Entra tenant.
type Tenant struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"displayName"`
	DefaultDomain string   `json:"defaultDomain"`
	Domains       []string `json:"domains"`
	CreatedAt     string   `json:"createdDateTime"`
}

// Directory provides access to directory data.
type Directory struct {
	st store.Store
	// Mu serialises read-modify-write sequences on directory objects.
	Mu sync.Mutex
}

// New returns a directory backed by st.
func New(st store.Store) *Directory { return &Directory{st: st} }

func objKey(tid, coll, id string) string {
	return strings.ToLower(tid + "/" + coll + "/" + id)
}

func deletedKey(tid, coll, id string) string {
	return strings.ToLower(tid + "/deleted/" + coll + "/" + id)
}

// ---------------------------------------------------------------- tenants

// PutTenant stores a tenant.
func (d *Directory) PutTenant(t Tenant) error {
	return store.PutJSON(d.st, store.BucketGraph, "_tenants/"+strings.ToLower(t.ID), t)
}

// Tenants lists all tenants.
func (d *Directory) Tenants() []Tenant {
	var out []Tenant
	_ = d.st.Scan(store.BucketGraph, "_tenants/", func(_ string, v []byte) bool {
		var t Tenant
		if json.Unmarshal(v, &t) == nil {
			out = append(out, t)
		}
		return true
	})
	return out
}

// ResolveTenant accepts a tenant GUID or verified domain name.
func (d *Directory) ResolveTenant(idOrDomain string) (Tenant, bool) {
	for _, t := range d.Tenants() {
		if strings.EqualFold(t.ID, idOrDomain) {
			return t, true
		}
		for _, dom := range t.Domains {
			if strings.EqualFold(dom, idOrDomain) {
				return t, true
			}
		}
	}
	return Tenant{}, false
}

// ---------------------------------------------------------------- objects

// Get returns an object from a collection.
func (d *Directory) Get(tid, coll, id string) (Object, bool) {
	var o Object
	ok, err := store.GetJSON(d.st, store.BucketGraph, objKey(tid, coll, id), &o)
	if err != nil || !ok {
		return nil, false
	}
	return o, true
}

// Put stores an object (must carry "id").
func (d *Directory) Put(tid, coll string, o Object) error {
	id, _ := o["id"].(string)
	if id == "" {
		return fmt.Errorf("object without id")
	}
	o["@odata.type"] = ODataType(coll)
	return store.PutJSON(d.st, store.BucketGraph, objKey(tid, coll, id), o)
}

// Delete soft-deletes an object into deletedItems (users, groups, applications)
// or removes it permanently (service principals, as Graph does).
func (d *Directory) Delete(tid, coll, id string) bool {
	o, ok := d.Get(tid, coll, id)
	if !ok {
		return false
	}
	_, _ = d.st.Delete(store.BucketGraph, objKey(tid, coll, id))
	if coll != ServicePrincipals {
		o["deletedDateTime"] = Now()
		_ = store.PutJSON(d.st, store.BucketGraph, deletedKey(tid, coll, id), o)
	}
	d.removeReferences(tid, id)
	return true
}

// GetDeleted returns a soft-deleted object.
func (d *Directory) GetDeleted(tid, id string) (string, Object, bool) {
	for _, c := range []string{Users, Groups, Applications} {
		var o Object
		if ok, _ := store.GetJSON(d.st, store.BucketGraph, deletedKey(tid, c, id), &o); ok {
			return c, o, true
		}
	}
	return "", nil, false
}

// ListDeleted lists soft-deleted objects of a collection.
func (d *Directory) ListDeleted(tid, coll string) []Object {
	var out []Object
	_ = d.st.Scan(store.BucketGraph, strings.ToLower(tid+"/deleted/"+coll+"/"), func(_ string, v []byte) bool {
		var o Object
		if json.Unmarshal(v, &o) == nil {
			out = append(out, o)
		}
		return true
	})
	return out
}

// PurgeDeleted permanently removes a soft-deleted object.
func (d *Directory) PurgeDeleted(tid, id string) bool {
	c, _, ok := d.GetDeleted(tid, id)
	if !ok {
		return false
	}
	_, _ = d.st.Delete(store.BucketGraph, deletedKey(tid, c, id))
	return true
}

// RestoreDeleted moves a soft-deleted object back.
func (d *Directory) RestoreDeleted(tid, id string) (string, Object, bool) {
	c, o, ok := d.GetDeleted(tid, id)
	if !ok {
		return "", nil, false
	}
	_, _ = d.st.Delete(store.BucketGraph, deletedKey(tid, c, id))
	o["deletedDateTime"] = nil
	_ = d.Put(tid, c, o)
	return c, o, true
}

// List returns all objects of a collection sorted by id.
func (d *Directory) List(tid, coll string) []Object {
	var out []Object
	_ = d.st.Scan(store.BucketGraph, strings.ToLower(tid+"/"+coll+"/"), func(_ string, v []byte) bool {
		var o Object
		if json.Unmarshal(v, &o) == nil {
			out = append(out, o)
		}
		return true
	})
	return out
}

// FindObject locates any directory object by id.
func (d *Directory) FindObject(tid, id string) (string, Object, bool) {
	for _, c := range AllCollections {
		if o, ok := d.Get(tid, c, id); ok {
			return c, o, true
		}
	}
	return "", nil, false
}

// FindBy returns the first object whose string field equals value (case-insensitive).
func (d *Directory) FindBy(tid, coll, field, value string) (Object, bool) {
	for _, o := range d.List(tid, coll) {
		if s, _ := o[field].(string); strings.EqualFold(s, value) {
			return o, true
		}
	}
	return nil, false
}

// TenantOfApp finds the tenant that holds a service principal for appID.
func (d *Directory) TenantOfApp(appID string) (Tenant, bool) {
	for _, t := range d.Tenants() {
		if _, ok := d.FindBy(t.ID, ServicePrincipals, "appId", appID); ok {
			return t, true
		}
	}
	return Tenant{}, false
}

// removeReferences drops a deleted object from owners/members lists.
func (d *Directory) removeReferences(tid, id string) {
	for _, c := range AllCollections {
		for _, o := range d.List(tid, c) {
			changed := false
			for _, rel := range []string{"_owners", "_members"} {
				ids := StringList(o[rel])
				if n := Remove(ids, id); len(n) != len(ids) {
					o[rel] = n
					changed = true
				}
			}
			if changed {
				_ = d.Put(tid, c, o)
			}
		}
	}
}

// ---------------------------------------------------------------- groups

// TransitiveGroups returns ids of all groups the object is (transitively) a member of.
func (d *Directory) TransitiveGroups(tid, objectID string) []string {
	groups := d.List(tid, Groups)
	seen := map[string]bool{}
	frontier := []string{strings.ToLower(objectID)}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		for _, g := range groups {
			gid := strings.ToLower(g["id"].(string))
			if seen[gid] {
				continue
			}
			for _, m := range StringList(g["_members"]) {
				if strings.EqualFold(m, cur) {
					seen[gid] = true
					frontier = append(frontier, gid)
					break
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- credentials

// HashSecret hashes a client secret for storage.
func HashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// VerifyClientSecret checks a secret against an application's password credentials.
// It returns the application object when the secret matches a current credential.
func (d *Directory) VerifyClientSecret(tid, appID, secret string) (Object, bool) {
	app, ok := d.FindBy(tid, Applications, "appId", appID)
	if !ok {
		return nil, false
	}
	want := HashSecret(secret)
	hashes, _ := app["_secretHashes"].(map[string]any)
	now := time.Now()
	creds := append(ObjectList(app["passwordCredentials"]), ObjectList(app["_spCredentials"])...)
	for _, pc := range creds {
		keyID, _ := pc["keyId"].(string)
		h, _ := hashes[keyID].(string)
		if h == "" || subtle.ConstantTimeCompare([]byte(h), []byte(want)) != 1 {
			continue
		}
		if end, _ := pc["endDateTime"].(string); end != "" {
			if t, err := time.Parse(time.RFC3339, end); err == nil && now.After(t) {
				continue
			}
		}
		if start, _ := pc["startDateTime"].(string); start != "" {
			if t, err := time.Parse(time.RFC3339, start); err == nil && now.Before(t.Add(-5*time.Minute)) {
				continue
			}
		}
		return app, true
	}
	return app, false
}

// ---------------------------------------------------------------- helpers

// NewGUID returns a random RFC 4122 v4 GUID.
func NewGUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewSecret returns a random client secret in Entra's style.
func NewSecret() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789~._-"
	b := make([]byte, 40)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// Now returns the current time in Graph's timestamp format.
func Now() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// StringList converts a JSON array to []string.
func StringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ObjectList converts a JSON array to []map.
func ObjectList(v any) []map[string]any {
	arr, _ := v.([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Remove returns ids without id (case-insensitive).
func Remove(ids []string, id string) []string {
	out := make([]string, 0, len(ids))
	for _, x := range ids {
		if !strings.EqualFold(x, id) {
			out = append(out, x)
		}
	}
	return out
}

// Contains reports whether ids contains id (case-insensitive).
func Contains(ids []string, id string) bool {
	for _, x := range ids {
		if strings.EqualFold(x, id) {
			return true
		}
	}
	return false
}
