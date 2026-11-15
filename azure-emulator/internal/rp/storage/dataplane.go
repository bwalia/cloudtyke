package storage

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// object is a stored blob or file.
type object struct {
	Data         string            `json:"data"` // base64
	ContentType  string            `json:"contentType"`
	ETag         string            `json:"etag"`
	LastModified string            `json:"lastModified"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Size         int64             `json:"size"`
	IsDir        bool              `json:"isDir,omitempty"`
}

func storageError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-ms-error-code", code)
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><Error><Code>%s</Code><Message>%s
RequestId:%s
Time:%s</Message></Error>`, code, xmlEscape(msg), entra.NewGUID(), time.Now().UTC().Format(time.RFC3339Nano))
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// authorize authenticates an Entra ID bearer token for Storage and checks the
// RBAC data action at scope. Shared keys and SAS are not emulated yet.
func (rp *RP) authorize(w http.ResponseWriter, r *http.Request, acct map[string]any, action, scope string) (*auth.Identity, bool) {
	h := r.Header.Get("Authorization")
	switch {
	case strings.HasPrefix(h, "SharedKey ") || r.URL.Query().Get("sig") != "":
		if props(acct)["allowSharedKeyAccess"] == false {
			storageError(w, 403, "KeyBasedAuthenticationNotPermitted", "Key based authentication is not permitted on this storage account.")
		} else {
			storageError(w, 403, "AuthenticationFailed", "CloudTyke supports Microsoft Entra ID (OAuth) for the storage data plane; shared key and SAS arrive in a later release.")
		}
		return nil, false
	case h == "":
		w.Header().Set("WWW-Authenticate", `Bearer authorization_uri=`+rp.s.Cfg.URL("login")+`/common/oauth2/authorize resource_id=https://`+rp.s.Cfg.Host("storage"))
		storageError(w, 401, "NoAuthenticationInformation", "Server failed to authenticate the request. Please refer to the information in the www-authenticate header.")
		return nil, false
	}
	id, _, msg := rp.s.Auth.Authenticate(r, rp.s.Cfg.Host("storage"))
	if id == nil {
		if id2, _, _ := rp.s.Auth.Authenticate(r, "storage.azure.com"); id2 != nil {
			id = id2
		}
	}
	if id == nil {
		storageError(w, 401, "InvalidAuthenticationInfo", "Server failed to authenticate the request: "+msg)
		return nil, false
	}
	if strings.EqualFold(strOf(props(acct)["publicNetworkAccess"]), "Disabled") {
		storageError(w, 403, "AuthorizationFailure", "This request is not authorized to perform this operation. Public network access is disabled.")
		return nil, false
	}
	if !rp.s.RBAC.Allowed(id, action, scope, true) {
		storageError(w, 403, "AuthorizationPermissionMismatch", "This request is not authorized to perform this operation using this permission.")
		return nil, false
	}
	return id, true
}

func accountFromHost(host, suffix string) string {
	host = strings.ToLower(host)
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return strings.TrimSuffix(host, suffix)
}

func (rp *RP) getObject(key string) (object, bool) {
	var o object
	ok, err := store.GetJSON(rp.s.Store, store.BucketData, key, &o)
	return o, ok && err == nil
}

func etag() string {
	return `"0x` + strings.ToUpper(strings.ReplaceAll(entra.NewGUID(), "-", "")[:15]) + `"`
}

func httpDate() string { return time.Now().UTC().Format(http.TimeFormat) }

func writeObjectHeaders(w http.ResponseWriter, o object) {
	w.Header().Set("ETag", o.ETag)
	w.Header().Set("Last-Modified", o.LastModified)
	w.Header().Set("Content-Type", o.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
	w.Header().Set("x-ms-version", "2023-11-03")
	for k, v := range o.Metadata {
		w.Header().Set("x-ms-meta-"+k, v)
	}
}

func metadataFrom(r *http.Request) map[string]string {
	m := map[string]string{}
	for k, v := range r.Header {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "x-ms-meta-") && len(v) > 0 {
			m[strings.TrimPrefix(lk, "x-ms-meta-")] = v[0]
		}
	}
	return m
}

// ------------------------------------------------------------ Blob

// BlobDataPlane serves https://<account>.blob.storage.<domain>.
func (rp *RP) BlobDataPlane() http.Handler {
	suffix := ".blob." + rp.s.Cfg.Host("storage")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := accountFromHost(r.Host, suffix)
		acct, ok := rp.accountByName(name)
		if !ok {
			storageError(w, 404, "ResourceNotFound", "The specified storage account does not exist.")
			return
		}
		w.Header().Set("x-ms-request-id", entra.NewGUID())
		w.Header().Set("x-ms-version", "2023-11-03")
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		q := r.URL.Query()
		base := strOf(acct["id"]) + "/blobServices/default"
		switch {
		case parts[0] == "" && q.Get("comp") == "list":
			if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read", strOf(acct["id"])); !ok {
				return
			}
			var b strings.Builder
			fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?><EnumerationResults ServiceEndpoint="https://%s%s/"><Containers>`, name, suffix)
			for _, c := range rp.s.Children(base + "/containers") {
				fmt.Fprintf(&b, `<Container><Name>%s</Name><Properties><Last-Modified>%s</Last-Modified><Etag>%s</Etag><PublicAccess>%s</PublicAccess></Properties></Container>`,
					xmlEscape(strOf(c["name"])), httpDate(), strOf(c["etag"]), strings.ToLower(strOf(props(c)["publicAccess"])))
			}
			b.WriteString(`</Containers><NextMarker /></EnumerationResults>`)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, b.String())
		case len(parts) == 1 && q.Get("restype") == "container":
			rp.containerOp(w, r, acct, parts[0])
		case len(parts) == 2 && parts[1] != "":
			rp.blobOp(w, r, acct, parts[0], parts[1])
		default:
			storageError(w, 400, "InvalidUri", "The requested URI does not represent any resource on the server.")
		}
	})
}

func (rp *RP) containerOp(w http.ResponseWriter, r *http.Request, acct map[string]any, container string) {
	cid := strOf(acct["id"]) + "/blobServices/default/containers/" + container
	switch {
	case r.Method == http.MethodPut:
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/write", strOf(acct["id"])); !ok {
			return
		}
		if _, ok := rp.s.LoadResource(cid); ok {
			storageError(w, 409, "ContainerAlreadyExists", "The specified container already exists.")
			return
		}
		if !containerRe.MatchString(container) {
			storageError(w, 400, "InvalidResourceName", "The specified resource name contains invalid characters.")
			return
		}
		_ = rp.s.SaveResource(map[string]any{"id": cid, "name": container, "type": ns + "/storageAccounts/blobServices/containers", "etag": etag(),
			"properties": map[string]any{"publicAccess": "None", "lastModifiedTime": time.Now().UTC().Format(time.RFC3339), "provisioningState": "Succeeded"}})
		w.WriteHeader(201)
	case r.Method == http.MethodDelete:
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/delete", cid); !ok {
			return
		}
		rp.s.DeleteTree(cid)
		rp.dropPrefix("blob/" + strings.ToLower(strOf(acct["name"])) + "/" + container + "/")
		w.WriteHeader(202)
	case r.Method == http.MethodGet && r.URL.Query().Get("comp") == "list":
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read", cid); !ok {
			return
		}
		if _, ok := rp.s.LoadResource(cid); !ok {
			storageError(w, 404, "ContainerNotFound", "The specified container does not exist.")
			return
		}
		prefix := "blob/" + strings.ToLower(strOf(acct["name"])) + "/" + container + "/"
		filter := r.URL.Query().Get("prefix")
		var names []string
		objs := map[string]object{}
		_ = rp.s.Store.Scan(store.BucketData, prefix, func(k string, v []byte) bool {
			n := strings.TrimPrefix(k, prefix)
			if strings.HasPrefix(n, filter) {
				var o object
				if json.Unmarshal(v, &o) == nil {
					names = append(names, n)
					objs[n] = o
				}
			}
			return true
		})
		sort.Strings(names)
		var b strings.Builder
		fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?><EnumerationResults ContainerName="%s"><Prefix>%s</Prefix><Blobs>`, container, xmlEscape(filter))
		for _, n := range names {
			o := objs[n]
			fmt.Fprintf(&b, `<Blob><Name>%s</Name><Properties><Last-Modified>%s</Last-Modified><Etag>%s</Etag><Content-Length>%d</Content-Length><Content-Type>%s</Content-Type><BlobType>BlockBlob</BlobType><AccessTier>Hot</AccessTier><LeaseStatus>unlocked</LeaseStatus><LeaseState>available</LeaseState></Properties></Blob>`,
				xmlEscape(n), o.LastModified, o.ETag, o.Size, xmlEscape(o.ContentType))
		}
		b.WriteString(`</Blobs><NextMarker /></EnumerationResults>`)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, b.String())
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read", cid); !ok {
			return
		}
		c, ok := rp.s.LoadResource(cid)
		if !ok {
			storageError(w, 404, "ContainerNotFound", "The specified container does not exist.")
			return
		}
		w.Header().Set("ETag", strOf(c["etag"]))
		w.Header().Set("Last-Modified", httpDate())
		w.WriteHeader(200)
	default:
		storageError(w, 405, "UnsupportedHttpVerb", "The resource doesn't support the specified HTTP verb.")
	}
}

func (rp *RP) blobOp(w http.ResponseWriter, r *http.Request, acct map[string]any, container, blob string) {
	cid := strOf(acct["id"]) + "/blobServices/default/containers/" + container
	key := "blob/" + strings.ToLower(strOf(acct["name"])) + "/" + container + "/" + blob
	if _, ok := rp.s.LoadResource(cid); !ok {
		storageError(w, 404, "ContainerNotFound", "The specified container does not exist.")
		return
	}
	switch r.Method {
	case http.MethodPut:
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/write", cid); !ok {
			return
		}
		if t := r.Header.Get("x-ms-blob-type"); t != "" && t != "BlockBlob" {
			storageError(w, 400, "UnsupportedBlobType", "CloudTyke supports block blobs.")
			return
		}
		data, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		ct := r.Header.Get("x-ms-blob-content-type")
		if ct == "" {
			ct = r.Header.Get("Content-Type")
		}
		if ct == "" {
			ct = "application/octet-stream"
		}
		o := object{Data: base64.StdEncoding.EncodeToString(data), ContentType: ct, ETag: etag(), LastModified: httpDate(), Metadata: metadataFrom(r), Size: int64(len(data))}
		_ = store.PutJSON(rp.s.Store, store.BucketData, key, o)
		sum := md5.Sum(data)
		w.Header().Set("ETag", o.ETag)
		w.Header().Set("Last-Modified", o.LastModified)
		w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
		w.Header().Set("x-ms-request-server-encrypted", "true")
		w.WriteHeader(201)
	case http.MethodGet, http.MethodHead:
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read", cid); !ok {
			return
		}
		o, ok := rp.getObject(key)
		if !ok {
			storageError(w, 404, "BlobNotFound", "The specified blob does not exist.")
			return
		}
		writeObjectHeaders(w, o)
		w.Header().Set("x-ms-blob-type", "BlockBlob")
		if r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		data, _ := base64.StdEncoding.DecodeString(o.Data)
		w.WriteHeader(200)
		_, _ = w.Write(data)
	case http.MethodDelete:
		if _, ok := rp.authorize(w, r, acct, "Microsoft.Storage/storageAccounts/blobServices/containers/blobs/delete", cid); !ok {
			return
		}
		if _, ok := rp.getObject(key); !ok {
			storageError(w, 404, "BlobNotFound", "The specified blob does not exist.")
			return
		}
		_, _ = rp.s.Store.Delete(store.BucketData, key)
		w.WriteHeader(202)
	default:
		storageError(w, 405, "UnsupportedHttpVerb", "The resource doesn't support the specified HTTP verb.")
	}
}

// ------------------------------------------------------------ Files

// FileDataPlane serves https://<account>.file.storage.<domain> (REST, OAuth).
func (rp *RP) FileDataPlane() http.Handler {
	suffix := ".file." + rp.s.Cfg.Host("storage")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := accountFromHost(r.Host, suffix)
		acct, ok := rp.accountByName(name)
		if !ok {
			storageError(w, 404, "ResourceNotFound", "The specified storage account does not exist.")
			return
		}
		w.Header().Set("x-ms-request-id", entra.NewGUID())
		w.Header().Set("x-ms-version", "2023-11-03")
		path := strings.Trim(r.URL.Path, "/")
		share, rest, _ := strings.Cut(path, "/")
		if share == "" {
			storageError(w, 400, "InvalidUri", "Specify a share.")
			return
		}
		if _, ok := rp.share(acct, share); !ok {
			storageError(w, 404, "ShareNotFound", "The specified share does not exist.")
			return
		}
		scope := strOf(acct["id"]) + "/fileServices/default/shares/" + share
		key := "file/" + strings.ToLower(strOf(acct["name"])) + "/" + share + "/" + rest
		q := r.URL.Query()
		action := func(verb string) string {
			return "Microsoft.Storage/storageAccounts/fileServices/fileshares/files/" + verb
		}
		switch {
		case q.Get("restype") == "directory" && r.Method == http.MethodPut:
			if _, ok := rp.authorize(w, r, acct, action("write"), scope); !ok {
				return
			}
			if _, ok := rp.getObject(key + "/"); ok {
				storageError(w, 409, "ResourceAlreadyExists", "The specified resource already exists.")
				return
			}
			_ = store.PutJSON(rp.s.Store, store.BucketData, key+"/", object{IsDir: true, ETag: etag(), LastModified: httpDate()})
			w.WriteHeader(201)
		case q.Get("restype") == "directory" && q.Get("comp") == "list":
			if _, ok := rp.authorize(w, r, acct, action("read"), scope); !ok {
				return
			}
			dir := key
			if rest != "" {
				dir += "/"
			}
			var b strings.Builder
			fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?><EnumerationResults ShareName="%s" DirectoryPath="%s"><Entries>`, share, xmlEscape(rest))
			var entries []string
			kind := map[string]object{}
			_ = rp.s.Store.Scan(store.BucketData, dir, func(k string, v []byte) bool {
				n := strings.TrimPrefix(k, dir)
				if n == "" {
					return true
				}
				first, more, nested := strings.Cut(n, "/")
				if nested && more != "" {
					return true // deeper level
				}
				var o object
				if json.Unmarshal(v, &o) == nil {
					entries = append(entries, first)
					kind[first] = o
				}
				return true
			})
			sort.Strings(entries)
			for _, n := range entries {
				if o := kind[n]; o.IsDir {
					fmt.Fprintf(&b, `<Directory><Name>%s</Name><Properties /></Directory>`, xmlEscape(n))
				} else {
					fmt.Fprintf(&b, `<File><Name>%s</Name><Properties><Content-Length>%d</Content-Length></Properties></File>`, xmlEscape(n), o.Size)
				}
			}
			b.WriteString(`</Entries><NextMarker /></EnumerationResults>`)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, b.String())
		case r.Method == http.MethodPut && q.Get("comp") == "range":
			if _, ok := rp.authorize(w, r, acct, action("write"), scope); !ok {
				return
			}
			o, ok := rp.getObject(key)
			if !ok {
				storageError(w, 404, "ResourceNotFound", "The specified resource does not exist.")
				return
			}
			var start int64
			rg := r.Header.Get("x-ms-range")
			if rg == "" {
				rg = r.Header.Get("Range")
			}
			if s, _, ok := strings.Cut(strings.TrimPrefix(rg, "bytes="), "-"); ok {
				start, _ = strconv.ParseInt(s, 10, 64)
			}
			data, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
			cur, _ := base64.StdEncoding.DecodeString(o.Data)
			if int64(len(cur)) < o.Size {
				cur = append(cur, make([]byte, o.Size-int64(len(cur)))...)
			}
			end := start + int64(len(data))
			if end > int64(len(cur)) {
				cur = append(cur, make([]byte, end-int64(len(cur)))...)
			}
			copy(cur[start:end], data)
			o.Data, o.ETag, o.LastModified = base64.StdEncoding.EncodeToString(cur), etag(), httpDate()
			_ = store.PutJSON(rp.s.Store, store.BucketData, key, o)
			w.Header().Set("ETag", o.ETag)
			w.WriteHeader(201)
		case r.Method == http.MethodPut:
			if _, ok := rp.authorize(w, r, acct, action("write"), scope); !ok {
				return
			}
			size, _ := strconv.ParseInt(r.Header.Get("x-ms-content-length"), 10, 64)
			ct := r.Header.Get("x-ms-content-type")
			if ct == "" {
				ct = "application/octet-stream"
			}
			o := object{ContentType: ct, ETag: etag(), LastModified: httpDate(), Metadata: metadataFrom(r), Size: size}
			_ = store.PutJSON(rp.s.Store, store.BucketData, key, o)
			w.Header().Set("ETag", o.ETag)
			w.WriteHeader(201)
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			if _, ok := rp.authorize(w, r, acct, action("read"), scope); !ok {
				return
			}
			o, ok := rp.getObject(key)
			if !ok || o.IsDir {
				storageError(w, 404, "ResourceNotFound", "The specified resource does not exist.")
				return
			}
			writeObjectHeaders(w, o)
			w.Header().Set("x-ms-type", "File")
			if r.Method == http.MethodHead {
				w.WriteHeader(200)
				return
			}
			data, _ := base64.StdEncoding.DecodeString(o.Data)
			if int64(len(data)) < o.Size {
				data = append(data, make([]byte, o.Size-int64(len(data)))...)
			}
			w.WriteHeader(200)
			_, _ = w.Write(data)
		case r.Method == http.MethodDelete:
			if _, ok := rp.authorize(w, r, acct, action("delete"), scope); !ok {
				return
			}
			_, _ = rp.s.Store.Delete(store.BucketData, key)
			w.WriteHeader(202)
		default:
			storageError(w, 405, "UnsupportedHttpVerb", "The resource doesn't support the specified HTTP verb.")
		}
	})
}
