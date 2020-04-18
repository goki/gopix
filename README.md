# GoPix

Go picture management app, because Mac Photos bricked my entire photo library upon updating to Catalina.

# Goals:

* simple, entirely file-based design -- no databases.   just write to exif meta data on images themselves.  so everything is future-proof and no lockin.

* sync with e.g., google drive using the drive sync thing.

* use gimp for photo retouching, or preview

# TODO

* set grid size based on allocated size at start of layout

* thumb and icon renders -- use icons for tree view -- needs an svg image node?

* click on image grid items, and scroll wheel for ImgGrid

* set symlink for dragging in treeview into folders

