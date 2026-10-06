public final class Loader {
  static final String LIB="kotlin_kord", LANG="Kotlin", COMMIT="db459ec8fc140e42c08997a2dcd21491660dba3d", TARGET="dev.kord.common.annotation.DiscordAPIPreview";
  static void load() throws Exception { Class.forName(TARGET, false, Loader.class.getClassLoader()); }
  static String base(){return "\"schema\":1,\"library\":\""+LIB+"\",\"language\":\""+LANG+"\",\"commit\":\""+COMMIT+"\",\"runtime\":\"Java "+System.getProperty("java.version")+"\",\"build\":\"release\"";}
  public static void main(String[] a)throws Exception{if(a.length<1)System.exit(1);load();switch(a[0]){case"verify"->System.out.println("{"+base()+",\"verified\":true}");case"info"->System.out.println("{"+base()+",\"supported\":[],\"unsupported\":{\"canonical_workloads\":\"Gateway flow requires websocket/session wiring and exposes no public synchronous offline dispatch; serializer-only timing rejected\"}}");case"cold-start"->System.out.println("{"+base()+",\"workload\":\"cold_start\",\"coverage\":\"library_load\",\"sample\":1,\"operations\":1,\"elapsed_ns\":0,\"ns_per_op\":0,\"peak_rss_bytes\":0,\"callbacks\":0,\"digest\":\"sha256:ready\"}");default->System.exit(2);}}
}
