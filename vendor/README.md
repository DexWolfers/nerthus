<!--2017-09-29 create by ysqi  -->

# 包管理器 

使用[GoVendor](https://github.com/kardianos/govendor)进行包管理，并将第三方包加入Git中，而非忽略。

在引入新的第三方包时，需要同步的将包加入包管理器，执行方式：
```shell
govendor update +e
```
`+e`是参数Status Types中的`+external`的缩写，具体见下文。
> +external (e) referenced packages in GOPATH but not in current project 

或者用`govendor get `替代`go get`。
```shell
govendor get golang.org/x/net
```
> govendor command: get      Like "go get" but copies dependencies into a "vendor" folder.


## 如何保证第三方包版本更新？
需要使用govendor 的命令`sync`执行更新，如果有变化将会更新到vendor目录，此变化应commit。
```shell
govendor sync +e
```
将从远程分支更新对应版本到vendor下。

## 为何不使用官方的dep？

未来也许会迁移到Go官方开发提供的包管理器[dep](https://github.com/golang/dep)。 但在尝试使用 dep时出现些麻烦。

在初始化时i/o timeout:
```shell
$ dep init
unable to deduce repository and source type for "golang.org/x/crypto/pbkdf2": unable to read metadata: unable to fetch raw metadata: failed HTTP request to URL "http://golang.org/x/crypto/pbkdf2?go-get=1": Get http://golang.org/x/crypto/pbkdf2?go-get=1: dial tcp 216.239.37.1:80: i/o timeout
```
尝试多种方式解决GFW下，如何顺利安装包的问题，其中有考虑对Git进行代理处理，但效果不佳。

```shell
git config --global http.https://golang.org.proxy socks5://127.0.0.1:1080
cat ~/.gitconfig
```
```yaml
[http "https://golang.org"]
	proxy = socks5://127.0.0.1:1080
```
还是无法总绕过GFW，而dep下也尚未对此进行特定开发。故现在不考虑使用dep。当下能快速使用即可。

## 为什么要将vendor纳入git？
还是GFW问题，在使用Docker进行自动测试与检查时不被GWF所困扰。可通过git clone 时便能将所依赖的包一次性下载，方便你我他。

同时新的开发人员加入，也无需多次`go get`获取依赖包。当然缺点是使得项目代码变大，但此问题仅是暂时的，未来会变得更美好。

未来也许会迁移到Go官方开发提供的包管理器[dep](https://github.com/golang/dep)，但当下，不过多考虑包管理使用的问题。

## Go Vendor Help 
govendor (v1.0.8): record dependencies and copy into vendor folder

### Commands:

+ -govendor-licenses    Show govendor's licenses.
+ -version              Show govendor version
+ -cpuprofile 'file'    Writes a CPU profile to 'file' for debugging.
+ -memprofile 'file'    Writes a heap profile to 'file' for debugging.

### Sub-Commands

+	init    <br> Create the "vendor" folder and the "vendor.json" file.
+	list    <br> List and filter existing dependencies and packages.
+	add     <br> Add packages from $GOPATH.
+	update  <br> Update packages from $GOPATH.
+	remove  <br> Remove packages from the vendor folder.
+	status  <br> Lists any packages missing, out-of-date, or modified locally.
+	fetch   <br> Add new or update vendor folder packages from remote repository.
+	sync    <br> Pull packages into vendor folder from remote repository with revisions from vendor.json file.
+	migrate  Move packages from a legacy tool to the vendor folder with metadata.
+	get      <br>Like "go get" but copies dependencies into a "vendor" folder.
+	license  <br>List discovered licenses for the given status or import paths.
+	shell    <br>Run a "shell" to make multiple sub-commands more efficient for large projects.
+	go tool commands that are wrapped: <br>"+status" package selection may be used with them fmt, build, install, clean, test, vet, generate, tool

### Status Types

+	+local    (l) <br>packages in your project
+	+external (e) <br>referenced packages in GOPATH but not in current project
+	+vendor   (v) <br>packages in the vendor folder
+	+std      (s) <br>packages in the standard library
+	+excluded (x) <br>external packages explicitly excluded from vendoring
+	+unused   (u) <br>packages in the vendor folder, but unused
+	+missing  (m) <br>referenced packages but not found
+	+program  (p) <br>package is a main package
+	+outside  +external +missing
+	+all      +all <br>packages 
 