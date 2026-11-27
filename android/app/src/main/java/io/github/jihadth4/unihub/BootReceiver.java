package io.github.jihadth4.unihub;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

/** Starts the engine again after the phone restarts or after the app was updated. */
public class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        String a = intent.getAction();
        if (Intent.ACTION_BOOT_COMPLETED.equals(a) || Intent.ACTION_MY_PACKAGE_REPLACED.equals(a)) {
            EngineService.start(context);
        }
    }
}
