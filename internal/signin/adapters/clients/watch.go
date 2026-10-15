package clients

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Resource is the cluster-scoped Client kind deploy-kit renders (deploy/crd/).
func Resource() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "identity.jorisjonkers.dev", Version: "v1", Resource: "clients"}
}

// object is a Client object as the API server stores it.
type object struct {
	Spec struct {
		SignIn       string   `json:"signIn"`
		Host         string   `json:"host"`
		Label        string   `json:"label"`
		Description  string   `json:"description"`
		LaunchURL    string   `json:"launchUrl"`
		IconURL      string   `json:"iconUrl"`
		RedirectURIs []string `json:"redirectUris"`
		Credential   string   `json:"credential"`
		SecretPath   string   `json:"secretPath"`
		Roles        []string `json:"roles"`
	} `json:"spec"`
}

var errNotAClient = errors.New("not a Client object")

// fromObject reads a Client out of an object; its id is the object's name.
func fromObject(obj any) (domain.Client, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return domain.Client{}, errNotAClient
	}
	var o object
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &o); err != nil {
		return domain.Client{}, err
	}
	s := o.Spec
	return domain.Client{
		ID: u.GetName(), SignIn: domain.SignInMode(s.SignIn), Host: s.Host, Label: s.Label, Description: s.Description,
		LaunchURL: s.LaunchURL, IconURL: s.IconURL, RedirectURIs: slices.Clone(s.RedirectURIs),
		Credential: domain.Credential(s.Credential), SecretPath: s.SecretPath, Roles: slices.Clone(s.Roles),
	}, nil
}

// Watch keeps x current from the cluster's Client objects until ctx ends, and returns whether the
// first full list has arrived; until it has, x may be missing Clients that exist.
func Watch(ctx context.Context, client dynamic.Interface, x *Index, logger *slog.Logger) (synced func() bool) {
	informer := dynamicinformer.NewDynamicSharedInformerFactory(client, 0).ForResource(Resource()).Informer()
	// Adding a handler fails only once an informer has stopped, and this one has not started.
	_, _ = informer.AddEventHandler(handlers(ctx, x, logger))
	go informer.RunWithContext(ctx)
	return informer.HasSynced
}

func handlers(ctx context.Context, x *Index, logger *slog.Logger) cache.ResourceEventHandlerFuncs {
	put := func(obj any) {
		c, err := fromObject(obj)
		if err != nil {
			logger.WarnContext(ctx, "an unreadable Client object", "error", err)
			return
		}
		x.Put(c)
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    put,
		UpdateFunc: func(_, obj any) { put(obj) },
		DeleteFunc: func(obj any) {
			if gone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = gone.Obj
			}
			if o, ok := obj.(interface{ GetName() string }); ok {
				x.Remove(o.GetName())
			}
		},
	}
}
