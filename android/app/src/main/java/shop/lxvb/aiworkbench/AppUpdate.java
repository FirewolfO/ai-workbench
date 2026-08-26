package shop.lxvb.aiworkbench;

import org.json.JSONObject;

final class AppUpdate {
    final String version;
    final String filename;
    final String url;
    final long size;

    private AppUpdate(String version, String filename, String url, long size) {
        this.version = version;
        this.filename = filename;
        this.url = url;
        this.size = size;
    }

    static AppUpdate from(JSONObject json) {
        return new AppUpdate(
                json.optString("version"),
                json.optString("filename"),
                json.optString("downloadUrl"),
                json.optLong("size")
        );
    }

    boolean isValid() {
        return !version.isEmpty() && !filename.isEmpty() && !url.isEmpty();
    }

    static boolean isNewer(String candidate, String current) {
        String[] candidateParts = candidate.split("\\.");
        String[] currentParts = current.split("\\.");
        int length = Math.max(candidateParts.length, currentParts.length);
        for (int index = 0; index < length; index++) {
            int candidatePart = parsePart(candidateParts, index);
            int currentPart = parsePart(currentParts, index);
            if (candidatePart != currentPart) return candidatePart > currentPart;
        }
        return false;
    }

    private static int parsePart(String[] parts, int index) {
        if (index >= parts.length) return 0;
        String numeric = parts[index].replaceFirst("[^0-9].*$", "");
        if (numeric.isEmpty()) return 0;
        try {
            return Integer.parseInt(numeric);
        } catch (NumberFormatException ignored) {
            return 0;
        }
    }
}
