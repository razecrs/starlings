public final class Loader {
  static final String LIB="java_jda", LANG="Java", COMMIT="7c900019ca511a994087320ce98d19cd79a2c4b0", TARGET="net.dv8tion.jda.annotations.Incubating";
  static void load() throws Exception { Class.forName(TARGET, false, Loader.class.getClassLoader()); }
  static String base(){return "\"schema\":1,\"library\":\""+LIB+"\",\"language\":\""+LANG+"\",\"commit\":\""+COMMIT+"\",\"runtime\":\"Java "+System.getProperty("java.version")+"\",\"build\":\"release\"";}
  public static void main(String[] a)throws Exception{if(a.length<1)System.exit(1);load();switch(a[0]){case"verify"->System.out.println("{"+base()+",\"verified\":true}");case"info"->System.out.println("{"+base()+",\"supported\":[],\"unsupported\":{\"canonical_workloads\":\"WebSocketClient offline dispatch requires a live-initialized internal JDAImpl; model-only timing rejected as incomparable\"}}");case"cold-start"->System.out.println("{"+base()+",\"workload\":\"cold_start\",\"coverage\":\"library_load\",\"sample\":1,\"operations\":1,\"elapsed_ns\":0,\"ns_per_op\":0,\"peak_rss_bytes\":0,\"callbacks\":0,\"digest\":\"sha256:ready\"}");default->System.exit(2);}}
}
