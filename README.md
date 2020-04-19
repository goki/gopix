# GoPix

Go picture management app, because Mac Photos bricked my entire photo library upon updating to Catalina.

# Goals

* simple, entirely file-based design -- no databases.   just write to exif meta data on images themselves.  so everything is future-proof and no lockin.

* sync with e.g., google drive using the drive sync thing.

* use gimp for photo retouching, or preview

# File Structure

* Main dir: `~/Pix`
* All images live in one common directory: `~/Pix/All`
* Folders for specific albums have symbolic links to original pics in `../All/img.jpg`
* DND / Copy/Paste operations create these symlinks, except if going into the trash -- or all

# TODO

* first pass read from AllInfo map in one thread, then parallel fill in rest -- lock contention highly inefficient

* rotate images

* write modified image with exif intact

