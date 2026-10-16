package rpc

import (
	"context"
	"fmt"
	"io"

	"github.com/yunj-yun/frp-web-panel/common"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func CallClientWrapper[R common.RespType](c *app.Context, clientID string, event pb.Event, req proto.Message, resp *R) error {
	cresp, err := CallClient(c, clientID, event, req)
	if err != nil {
		return err
	}

	protoMsgRef, ok := any(resp).(protoreflect.ProtoMessage)
	if !ok {
		return fmt.Errorf("type does not implement protoreflect.ProtoMessage")
	}

	return proto.Unmarshal(cresp.GetData(), protoMsgRef)
}

func CallClient(ctx *app.Context, clientID string, event pb.Event, msg proto.Message) (*pb.ClientMessage, error) {
	sender := ctx.GetApp().GetClientsManager().Get(clientID)
	if sender == nil {
		logger.Logger(ctx).Errorf("cannot get client, id: [%s]", clientID)
		return nil, fmt.Errorf("cannot get client, id: [%s]", clientID)
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Errorf("cannot marshal")
		return nil, err
	}

	req := &pb.ServerMessage{
		Event:     event,
		Data:      data,
		SessionId: uuid.New().String(),
		ClientId:  clientID,
	}

	ctx.GetApp().GetClientRecvMap().Store(req.SessionId, make(chan *pb.ClientMessage))
	err = sender.Conn.Send(req)
	if err != nil {
		logger.Logger(context.Background()).WithError(err).Errorf("cannot send")
		ctx.GetApp().GetClientsManager().Remove(clientID)
		return nil, err
	}
	respChAny, ok := ctx.GetApp().GetClientRecvMap().Load(req.SessionId)
	if !ok {
		logger.Logger(ctx).Fatalf("cannot load")
	}

	respCh, ok := respChAny.(chan *pb.ClientMessage)
	if !ok {
		logger.Logger(ctx).Fatalf("cannot cast")
	}

	resp := <-respCh
	if resp.Event == pb.Event_EVENT_ERROR {
		return nil, fmt.Errorf("client return error: %s", resp.Data)
	}

	close(respCh)
	ctx.GetApp().GetClientRecvMap().Delete(req.SessionId)
	return resp, nil
}

func Recv(appInstance app.Application, clientID string) chan bool {
	done := make(chan bool)
	go func() {
		c := context.Background()
		log := logger.Logger(c).WithField("clientID", clientID)
		for {
			reciver := appInstance.GetClientsManager().Get(clientID)
			if reciver == nil {
				log.Errorf("cannot get client")
				continue
			}
			resp, err := reciver.Conn.Recv()
			if err == io.EOF {
				log.Infof("finish client recv")
				done <- true
				return
			}
			if err != nil {
				log.WithError(err).Errorf("cannot recv, usually means client disconnect")
				done <- true
				return
			}

			respChAny, ok := appInstance.GetClientRecvMap().Load(resp.SessionId)
			if !ok {
				log.Errorf("cannot load")
				continue
			}

			respCh, ok := respChAny.(chan *pb.ClientMessage)
			if !ok {
				log.Errorf("cannot cast")
				continue
			}
			log.Debugf("recv success, clientID: [%s], event: [%s], sessionID: [%s]", resp.GetClientId(), resp.GetEvent().String(), resp.GetSessionId())
			respCh <- resp
		}
	}()
	return done
}
