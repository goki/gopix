# Mac install

app-install:
	/bin/cp $(HOME)/go/bin/gopix GoPix.app/Contents/MacOS/
	/bin/cp -av GoPix.app /Applications

dev-install:
	/bin/cp -av GoPix-dev.app /Applications/GoPix.app

app-dmg:
	/bin/cp $(HOME)/go/bin/gopix GoPix.app/Contents/MacOS/
	/usr/bin/hdiutil create -ov -srcfolder GoPix.app GoPix.dmg

