package main

func main() {
	// conn, err := grpc.Dial("127.0.0.1:9000", grpc.WithTransportCredentials(insecure.NewCredentials()))
	// if err != nil {
	// 	panic(err)
	// }

	// cl := v1.NewTwitchEdgeServiceClient(conn)
	// spew.Dump(cl.JoinChannel(context.Background(), &v1.JoinChannelRequest{
	// 	Channels: []*v1.Channel{{
	// 		Id:       "121903137",
	// 		Login:    "troykomodo",
	// 		Priority: 5001,
	// 	}, {
	// 		Id:       "503012119",
	// 		Login:    "7tvapp",
	// 		Priority: 100,
	// 	}, {
	// 		Id:       "614822347",
	// 		Login:    "komodotroy",
	// 		Priority: 1001,
	// 	}},
	// }))
	// stream, err := cl.RegisterEdge(context.Background(), &v1.RegisterEdgeRequest{
	// 	NodeName: "dummy-0",
	// })
	// if err != nil {
	// 	panic(err)
	// }

	// for {
	// 	msg, err := stream.Recv()
	// 	spew.Dump(msg, err)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }

	// ctx := context.Background()

	// wg := sync.WaitGroup{}
	// const total = 500000
	// wg.Add(total)
	// i2 := utils.PointerOf(int32(0))
	// printer := make(chan string, 10000)
	// for i := 0; i < total; i++ {
	// 	go func(i int) {
	// 		printer <- fmt.Sprint("loaded: ", atomic.AddInt32(i2, 1), "\n")

	// 		_, err := cl.JoinChannelEdge(ctx, &v1.JoinChannelEdgeRequest{
	// 			Channel: &v1.Channel{
	// 				Id:       "121903137",
	// 				Login:    "troykomodo",
	// 				Priority: int32(i),
	// 			},
	// 		})
	// 		if err != nil {
	// 			panic(err)
	// 		}

	// 		printer <- fmt.Sprintf("%d - %d\n", i, atomic.AddInt32(i2, -1))
	// 		wg.Done()
	// 	}(i)
	// }
	// go func() {
	// 	for line := range printer {
	// 		fmt.Print(line)
	// 	}
	// }()

	// wg.Wait()
}
