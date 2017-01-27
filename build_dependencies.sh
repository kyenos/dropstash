
echo "This script installs and builds the necessary dependencies for dropstash"
# Don't use it directly, use go generate for it to work correctly otherwise things
# might not get built in the correct order!!

if [ ! -e /usr/bin/patch ]; then
    echo ""
    echo "**** WARNING **** The patch utility is require to build dropstash"
    echo ""
    exit 1
fi

echo "Installing stringer if necessary"
go get golang.org/x/tools/cmd/stringer


if [ -e "$GOPATH/src/github.com/kardianos/osext" ]; then
    echo "Removing previous installation of osext"
    rm -rf `find $GOPATH |grep osext`
fi
echo "Installing osext"
go get github.com/kardianos/osext

if [ -e "$GOPATH/src/github.com/sevlyar/go-daemon" ]; then
    echo "Removing previous installation of go-daemon"
    rm -rf `find $GOPATH |grep go-daemon`
fi

echo "Installing go-daemon"
go get github.com/sevlyar/go-daemon
_xt=`pwd`
pushd "$GOPATH/src/github.com/sevlyar/go-daemon" > /dev/null 2>&1
patch -p0 < "$_xt/daemon.patch" >/dev/null 2>&1
git checkout v0.1.0
rm daemon_posix.go.orig
if [ -e "$GOPATH/pkg/linux_amd64/github.com/sevlyar/go-daemon.a" ]; then
    rm "$GOPATH/pkg/linux_amd64/github.com/sevlyar/go-daemon.a"
fi
go install
popd > /dev/null 2>&1

if [ -e "$GOPATH/src/github.com/Sirupsen/logrus" ]; then
    echo "Removing previous installation of logrus"
    rm -rf `find $GOPATH |grep logrus`
fi
echo "Installing logrus"
go get github.com/Sirupsen/logrus

if [ -e "$GOPATH/src/code.google.com/p/go-uuid/uuid" ]; then
    echo "Removing previous installation of go-uuid"
    rm -rf `find $GOPATH |grep go-uuid`
fi
if [ -e "$GOPATH/src/github.com/google/uuid" ]; then
    echo "Removing previous installation of go-uuid"
    rm -rf `find $GOPATH |grep gooogle\/uuid`
fi
echo "Installing go-uuid"
go get github.com/google/uuid

