package io.github.jihadth4.unihub;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.List;
import java.util.Locale;

/**
 * Lets other apps (PDF reader, PowerPoint, the package installer...) read files that belong
 * to this app, one file at a time and read-only, through content:// links. Only files under
 * the app's own folders can be shared.
 */
public class FilesProvider extends ContentProvider {

    static String authority(Context c) {
        return c.getPackageName() + ".files";
    }

    private static File[] roots(Context c) {
        return new File[]{Engine.rootBase(c), c.getFilesDir(), c.getCacheDir()};
    }

    /** content:// link for f, or null when f is outside the app's folders. */
    static Uri uriFor(Context c, File f) {
        try {
            String path = f.getCanonicalPath();
            File[] rs = roots(c);
            for (int i = 0; i < rs.length; i++) {
                String root = rs[i].getCanonicalPath();
                if (path.startsWith(root + File.separator)) {
                    Uri.Builder b = new Uri.Builder().scheme("content").authority(authority(c)).appendPath(String.valueOf(i));
                    for (String seg : path.substring(root.length() + 1).split("/")) b.appendPath(seg);
                    return b.build();
                }
            }
        } catch (IOException ignored) {
        }
        return null;
    }

    private File fileFor(Uri u) throws FileNotFoundException {
        List<String> segs = u.getPathSegments();
        if (segs.size() < 2) throw new FileNotFoundException();
        try {
            File[] rs = roots(getContext());
            int i = Integer.parseInt(segs.get(0));
            if (i < 0 || i >= rs.length) throw new FileNotFoundException();
            File f = rs[i];
            for (String s : segs.subList(1, segs.size())) f = new File(f, s);
            String root = rs[i].getCanonicalPath();
            if (!f.getCanonicalPath().startsWith(root + File.separator) || !f.isFile()) throw new FileNotFoundException();
            return f;
        } catch (IOException | NumberFormatException e) {
            throw new FileNotFoundException();
        }
    }

    static String mimeOf(String name) {
        int dot = name.lastIndexOf('.');
        String ext = dot >= 0 ? name.substring(dot + 1).toLowerCase(Locale.ROOT) : "";
        if (ext.equals("apk")) return "application/vnd.android.package-archive";
        String m = MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext);
        return m != null ? m : "application/octet-stream";
    }

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public String getType(Uri uri) {
        return mimeOf(uri.getLastPathSegment() == null ? "" : uri.getLastPathSegment());
    }

    @Override
    public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if (mode != null && !mode.equals("r")) throw new FileNotFoundException("read only");
        return ParcelFileDescriptor.open(fileFor(uri), ParcelFileDescriptor.MODE_READ_ONLY);
    }

    @Override
    public Cursor query(Uri uri, String[] projection, String selection, String[] args, String sort) {
        File f;
        try {
            f = fileFor(uri);
        } catch (FileNotFoundException e) {
            return null;
        }
        if (projection == null) projection = new String[]{OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE};
        MatrixCursor c = new MatrixCursor(projection, 1);
        Object[] row = new Object[projection.length];
        for (int i = 0; i < projection.length; i++) {
            if (OpenableColumns.DISPLAY_NAME.equals(projection[i])) row[i] = f.getName();
            else if (OpenableColumns.SIZE.equals(projection[i])) row[i] = f.length();
        }
        c.addRow(row);
        return c;
    }

    @Override
    public Uri insert(Uri uri, ContentValues values) {
        throw new UnsupportedOperationException();
    }

    @Override
    public int delete(Uri uri, String selection, String[] args) {
        throw new UnsupportedOperationException();
    }

    @Override
    public int update(Uri uri, ContentValues values, String selection, String[] args) {
        throw new UnsupportedOperationException();
    }
}
