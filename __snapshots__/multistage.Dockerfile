FROM busybox as builder
WORKDIR /go/src
RUN touch a.txt && touch b.txt
FROM busybox as builder2
WORKDIR /go/src
RUN touch b.txt
FROM busybox
WORKDIR /todo
COPY --from=builder2 /go/src/b.txt ./
COPY --from=builder /go/src/a.txt ./
