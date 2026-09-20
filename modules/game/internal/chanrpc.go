package internal

import "gameserver/common/session"

type playerSessionCloser interface {
	ClosePlayerSession(session.PlayerSession)
}

func registerCloseAgent(closer playerSessionCloser) {
	skeleton.RegisterChanRPC("CloseAgent", func(args []interface{}) {
		rpcCloseAgent(closer, args)
	})
}

func rpcCloseAgent(closer playerSessionCloser, args []interface{}) {
	if len(args) < 2 {
		return
	}
	snapshot, ok := session.FromData(args[1])
	if !ok {
		return
	}
	closer.ClosePlayerSession(snapshot)
}
