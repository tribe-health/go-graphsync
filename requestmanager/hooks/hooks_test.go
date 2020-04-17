package hooks_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/ipfs/go-graphsync"
	gsmsg "github.com/ipfs/go-graphsync/message"
	"github.com/ipfs/go-graphsync/requestmanager/hooks"
	"github.com/ipfs/go-graphsync/testutil"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

func TestRequestHookProcessing(t *testing.T) {
	fakeChooser := func(ipld.Link, ipld.LinkContext) (ipld.NodeBuilder, error) {
		return ipldfree.NodeBuilder(), nil
	}
	extensionData := testutil.RandomBytes(100)
	extensionName := graphsync.ExtensionName("AppleSauce/McGee")
	extension := graphsync.ExtensionData{
		Name: extensionName,
		Data: extensionData,
	}

	root := testutil.GenerateCids(1)[0]
	requestID := graphsync.RequestID(rand.Int31())
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	request := gsmsg.NewRequest(requestID, root, ssb.Matcher().Node(), graphsync.Priority(0), extension)
	p := testutil.GeneratePeers(1)[0]
	testCases := map[string]struct {
		configure func(t *testing.T, hooks *hooks.Hooks)
		assert    func(t *testing.T, result hooks.RequestResult)
	}{
		"no hooks": {
			assert: func(t *testing.T, result hooks.RequestResult) {
				require.Nil(t, result.CustomChooser)
				require.Empty(t, result.PersistenceOption)
			},
		},
		"hooks alter chooser": {
			configure: func(t *testing.T, hooks *hooks.Hooks) {
				hooks.RegisterRequestHook(func(p peer.ID, requestData graphsync.RequestData, hookActions graphsync.OutgoingRequestHookActions) {
					if _, found := requestData.Extension(extensionName); found {
						hookActions.UseNodeBuilderChooser(fakeChooser)
					}
				})
			},
			assert: func(t *testing.T, result hooks.RequestResult) {
				require.NotNil(t, result.CustomChooser)
				require.Empty(t, result.PersistenceOption)
			},
		},
		"hooks alter persistence option": {
			configure: func(t *testing.T, hooks *hooks.Hooks) {
				hooks.RegisterRequestHook(func(p peer.ID, requestData graphsync.RequestData, hookActions graphsync.OutgoingRequestHookActions) {
					if _, found := requestData.Extension(extensionName); found {
						hookActions.UsePersistenceOption("chainstore")
					}
				})
			},
			assert: func(t *testing.T, result hooks.RequestResult) {
				require.Nil(t, result.CustomChooser)
				require.Equal(t, "chainstore", result.PersistenceOption)
			},
		},
		"hooks unregistered": {
			configure: func(t *testing.T, hooks *hooks.Hooks) {
				unregister := hooks.RegisterRequestHook(func(p peer.ID, requestData graphsync.RequestData, hookActions graphsync.OutgoingRequestHookActions) {
					if _, found := requestData.Extension(extensionName); found {
						hookActions.UsePersistenceOption("chainstore")
					}
				})
				unregister()
			},
			assert: func(t *testing.T, result hooks.RequestResult) {
				require.Nil(t, result.CustomChooser)
				require.Empty(t, result.PersistenceOption)
			},
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			hooks := hooks.New()
			if data.configure != nil {
				data.configure(t, hooks)
			}
			result := hooks.ProcessRequestHooks(p, request)
			if data.assert != nil {
				data.assert(t, result)
			}
		})
	}
}

func TestResponseHookProcessing(t *testing.T) {

	extensionResponseData := testutil.RandomBytes(100)
	extensionName := graphsync.ExtensionName("AppleSauce/McGee")
	extensionResponse := graphsync.ExtensionData{
		Name: extensionName,
		Data: extensionResponseData,
	}
	extensionUpdateData := testutil.RandomBytes(100)
	extensionUpdate := graphsync.ExtensionData{
		Name: extensionName,
		Data: extensionUpdateData,
	}
	requestID := graphsync.RequestID(rand.Int31())
	response := gsmsg.NewResponse(requestID, graphsync.PartialResponse, extensionResponse)

	p := testutil.GeneratePeers(1)[0]
	testCases := map[string]struct {
		configure func(t *testing.T, hooks *hooks.Hooks)
		assert    func(t *testing.T, result hooks.ResponseResult)
	}{
		"no hooks": {
			assert: func(t *testing.T, result hooks.ResponseResult) {
				require.Empty(t, result.Extensions)
				require.NoError(t, result.Err)
			},
		},
		"short circuit on error": {
			configure: func(t *testing.T, hooks *hooks.Hooks) {
				hooks.RegisterResponseHook(func(p peer.ID, responseData graphsync.ResponseData, hookActions graphsync.IncomingResponseHookActions) {
					hookActions.TerminateWithError(errors.New("something went wrong"))
				})
				hooks.RegisterResponseHook(func(p peer.ID, responseData graphsync.ResponseData, hookActions graphsync.IncomingResponseHookActions) {
					hookActions.UpdateRequestWithExtensions(extensionUpdate)
				})
			},
			assert: func(t *testing.T, result hooks.ResponseResult) {
				require.Empty(t, result.Extensions)
				require.EqualError(t, result.Err, "something went wrong")
			},
		},
		"hooks update with extensions": {
			configure: func(t *testing.T, hooks *hooks.Hooks) {
				hooks.RegisterResponseHook(func(p peer.ID, responseData graphsync.ResponseData, hookActions graphsync.IncomingResponseHookActions) {
					if _, found := responseData.Extension(extensionName); found {
						hookActions.UpdateRequestWithExtensions(extensionUpdate)
					}
				})
			},
			assert: func(t *testing.T, result hooks.ResponseResult) {
				require.Len(t, result.Extensions, 1)
				require.Equal(t, extensionUpdate, result.Extensions[0])
				require.NoError(t, result.Err)
			},
		},
		"hooks unregistered": {
			configure: func(t *testing.T, hooks *hooks.Hooks) {
				unregister := hooks.RegisterResponseHook(func(p peer.ID, responseData graphsync.ResponseData, hookActions graphsync.IncomingResponseHookActions) {
					if _, found := responseData.Extension(extensionName); found {
						hookActions.UpdateRequestWithExtensions(extensionUpdate)
					}
				})
				unregister()
			},
			assert: func(t *testing.T, result hooks.ResponseResult) {
				require.Empty(t, result.Extensions)
				require.NoError(t, result.Err)
			},
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			hooks := hooks.New()
			if data.configure != nil {
				data.configure(t, hooks)
			}
			result := hooks.ProcessResponseHooks(p, response)
			if data.assert != nil {
				data.assert(t, result)
			}
		})
	}
}
