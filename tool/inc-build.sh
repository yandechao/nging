export DISTPATH=../dist
export OSVERSIONDIR=${NGING_EXECUTOR}_${GOOS}_${GOARCH}
if [ "$GOARM" != "" ]; then
	export OSVERSIONDIR=${OSVERSIONDIR}v${GOARM}
fi
export RELEASEDIR=${DISTPATH}/${OSVERSIONDIR}
mkdir ${RELEASEDIR}

export LDFLAGS="-extldflags '-static'"
case "$GOARCH" in
    "arm"|"arm64"|"arm-7"|"arm-6"|"arm-5")
        export LDFLAGS="-extldflags '-static'"
        ;;
    *)
        export LDFLAGS=""
esac

go build -tags "bindata sqlite${BUILDTAGS}" -ldflags="-X main.BUILD_TIME=${NGING_BUILD} -X main.COMMIT=${NGING_COMMIT} -X main.VERSION=${NGING_VERSION} -X main.LABEL=${NGING_LABEL} -X main.BUILD_OS=${GOOS} -X main.BUILD_ARCH=${GOARCH} ${MINIFYFLAG} ${LDFLAGS}" -o ${RELEASEDIR}/${NGING_EXECUTOR}${NGINGEX} ..

mkdir ${RELEASEDIR}/data
mkdir ${RELEASEDIR}/data/logs
cp -R ../data/ip2region ${RELEASEDIR}/data/ip2region


mkdir ${RELEASEDIR}/config
mkdir ${RELEASEDIR}/config/vhosts

#cp -R ../config/config.yaml ${RELEASEDIR}/config/config.yaml
cp -R ../config/config.yaml.sample ${RELEASEDIR}/config/config.yaml.sample
cp -R ../config/preupgrade.* ${RELEASEDIR}/config/
cp -R ../config/ua.txt ${RELEASEDIR}/config/ua.txt

export archiver_extension="tar.gz"

# if [ "$GOOS" = "windows" ]; then
# 	cp -R ../support/sqlite3_${GOARCH}.dll ${RELEASEDIR}/
# fi

cp -R ../dist/default/* ${RELEASEDIR}/

rm -rf ${RELEASEDIR}.${archiver_extension}

tar -zcvf ${RELEASEDIR}.${archiver_extension} -C ${DISTPATH} ${OSVERSIONDIR}

rm -rf ${RELEASEDIR}
